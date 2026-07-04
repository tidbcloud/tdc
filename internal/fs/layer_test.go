package fs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apifs "github.com/Icemap/tdc/internal/api/fs"
)

func TestCreateLayerRejectsDuplicateTag(t *testing.T) {
	_, err := testService(t.TempDir(), "https://fs.test").CreateLayer(context.Background(), CreateLayerOptions{
		Profile:      dataProfile(),
		BaseRootPath: "/repo",
		Tags:         []string{"task=auth", "task=review"},
	})
	if err == nil || !strings.Contains(err.Error(), `duplicate layer tag "task"`) {
		t.Fatalf("CreateLayer duplicate tag err=%v, want duplicate tag error", err)
	}
}

func TestCreateLayerSendsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/layers" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		var req apifs.FSLayerCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.LayerID != "layer-1" || req.BaseRootPath != "/repo" || req.Name != "task" || req.DurabilityMode != "restore-safe" {
			t.Fatalf("request = %#v", req)
		}
		if req.Tags["task"] != "auth" || req.Tags["env"] != "dev" {
			t.Fatalf("tags = %#v", req.Tags)
		}
		_ = json.NewEncoder(w).Encode(apifs.FSLayer{LayerID: "layer-1", BaseRootPath: req.BaseRootPath, Name: req.Name, State: "active", DurabilityMode: req.DurabilityMode})
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).CreateLayer(context.Background(), CreateLayerOptions{
		Profile:        dataProfile(),
		LayerID:        "layer-1",
		BaseRootPath:   "/repo",
		LayerName:      "task",
		Tags:           []string{"task=auth", "env=dev"},
		DurabilityMode: "restore-safe",
	})
	if err != nil {
		t.Fatalf("CreateLayer: %v", err)
	}
	if result.LayerID != "layer-1" || result.State != "active" {
		t.Fatalf("result = %#v", result)
	}
}

func TestCopyFileLocalToLayerUsesLayerObjectUpload(t *testing.T) {
	localPath := filepath.Join(t.TempDir(), "layer.txt")
	if err := os.WriteFile(localPath, []byte("layer upload"), 0o640); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/layers/layer-1/objects" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		if r.URL.Query().Get("path") != "/repo/layer.txt" || r.URL.Query().Get("mode") != "640" {
			t.Fatalf("unexpected query %q", r.URL.RawQuery)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "layer upload" {
			t.Fatalf("body = %q", body)
		}
		_ = json.NewEncoder(w).Encode(apifs.FSLayerEntry{
			LayerID:   "layer-1",
			Path:      "/repo/layer.txt",
			Op:        "upsert",
			Kind:      "file",
			SizeBytes: int64(len(body)),
			Mode:      0o640,
			EntrySeq:  2,
		})
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).CopyFile(context.Background(), CopyFileOptions{
		Profile:   dataProfile(),
		FromLocal: localPath,
		ToRemote:  "/repo/layer.txt",
		LayerID:   "layer-1",
	})
	if err != nil {
		t.Fatalf("CopyFile --layer-id: %v", err)
	}
	if result.Operation != "copy_file_to_layer" || result.Status != "layered" || result.Revision != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestSearchAndFindPassLayerID(t *testing.T) {
	var grepLayer string
	var findLayer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/repo" && hasQueryKey(r.URL.RawQuery, "grep"):
			grepLayer = r.URL.Query().Get("layer")
			_ = json.NewEncoder(w).Encode([]apifs.SearchResult{{Path: "/repo/a.txt"}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/repo" && hasQueryKey(r.URL.RawQuery, "find"):
			findLayer = r.URL.Query().Get("layer")
			_ = json.NewEncoder(w).Encode([]apifs.SearchResult{{Path: "/repo/a.txt"}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	service := testService(t.TempDir(), server.URL)
	if _, err := service.SearchFileContent(context.Background(), SearchFileContentOptions{Profile: dataProfile(), Path: "/repo", Pattern: "needle", LayerID: "layer-1"}); err != nil {
		t.Fatalf("SearchFileContent: %v", err)
	}
	if _, err := service.FindFiles(context.Background(), FindFilesOptions{Profile: dataProfile(), Path: "/repo", FileNamePattern: "*.txt", LayerID: "layer-1"}); err != nil {
		t.Fatalf("FindFiles: %v", err)
	}
	if grepLayer != "layer-1" || findLayer != "layer-1" {
		t.Fatalf("grepLayer=%q findLayer=%q", grepLayer, findLayer)
	}
}
