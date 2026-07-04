package fs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFSLayerClientMethods(t *testing.T) {
	var sawUpload bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/layers":
			var req FSLayerCreateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode create layer request: %v", err)
			}
			if req.BaseRootPath != "/repo" || req.Name != "task" || req.Tags["task"] != "auth" {
				t.Fatalf("unexpected create layer request: %#v", req)
			}
			_ = json.NewEncoder(w).Encode(FSLayer{LayerID: "layer-1", BaseRootPath: req.BaseRootPath, Name: req.Name, State: "active"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/layers":
			_ = json.NewEncoder(w).Encode(map[string]any{"layers": []FSLayer{{LayerID: "layer-1", State: "active"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/layers/layer-1":
			_ = json.NewEncoder(w).Encode(FSLayer{LayerID: "layer-1", State: "active"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/layers/layer-1/diff":
			if r.URL.Query().Get("max_seq") == "7" && r.URL.Query().Get("replay") != "1" {
				_ = json.NewEncoder(w).Encode(map[string]any{"entries": []FSLayerEntry{{LayerID: "layer-1", Path: "/repo/a.txt", EntrySeq: 7}}})
				return
			}
			if r.URL.Query().Get("replay") == "1" {
				_ = json.NewEncoder(w).Encode(map[string]any{"entries": []FSLayerEntry{{LayerID: "layer-1", Path: "/repo/replay.txt", EntrySeq: 3}}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"entries": []FSLayerEntry{{LayerID: "layer-1", Path: "/repo/a.txt", EntrySeq: 1}}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/layers/layer-1/entries":
			var req FSLayerEntryRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode layer entry request: %v", err)
			}
			if req.Path != "/repo/a.txt" || req.Op != "upsert" || string(req.Content) != "hello" {
				t.Fatalf("unexpected layer entry request: %#v", req)
			}
			_ = json.NewEncoder(w).Encode(FSLayerEntry{LayerID: "layer-1", Path: req.Path, Op: req.Op, EntrySeq: 2})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/layers/layer-1/objects":
			sawUpload = true
			if r.URL.Query().Get("path") != "/repo/upload.txt" || r.URL.Query().Get("mode") != "640" || r.URL.Query().Get("base_revision") != "2" {
				t.Fatalf("unexpected upload query: %s", r.URL.RawQuery)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read upload body: %v", err)
			}
			if string(body) != "upload bytes" {
				t.Fatalf("unexpected upload body %q", body)
			}
			_ = json.NewEncoder(w).Encode(FSLayerEntry{LayerID: "layer-1", Path: "/repo/upload.txt", EntrySeq: 4, SizeBytes: int64(len(body))})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/layers/layer-1/objects":
			if r.URL.Query().Get("path") != "/repo/upload.txt" || r.URL.Query().Get("max_seq") != "4" {
				t.Fatalf("unexpected read layer object query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte("upload bytes"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/layers/layer-1/entries":
			if r.URL.Query().Get("path") != "/repo/a.txt" {
				t.Fatalf("unexpected get entry query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(FSLayerEntry{LayerID: "layer-1", Path: "/repo/a.txt", EntrySeq: 2})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/layers/layer-1/checkpoints":
			_ = json.NewEncoder(w).Encode(FSLayerCheckpoint{CheckpointID: "cp-1", LayerID: "layer-1", DurableSeq: 4})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/layer-checkpoints/cp-1":
			_ = json.NewEncoder(w).Encode(FSLayerCheckpoint{CheckpointID: "cp-1", LayerID: "layer-1", DurableSeq: 4})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/layers/layer-1/events":
			if r.URL.Query().Get("since") != "3" {
				t.Fatalf("unexpected events query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"events": []FSLayerEvent{{LayerID: "layer-1", Seq: 4, Op: "upsert", Path: "/repo/a.txt"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/layers/layer-1/rollback":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/layers/layer-1/commit":
			_ = json.NewEncoder(w).Encode(FSLayerCommit{Status: "committed", LayerID: "layer-1", Applied: 1})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	client := testClient(t, server.URL)
	layer, err := client.CreateFSLayer(context.Background(), FSLayerCreateRequest{BaseRootPath: "/repo", Name: "task", Tags: map[string]string{"task": "auth"}})
	if err != nil || layer.LayerID != "layer-1" {
		t.Fatalf("CreateFSLayer = %#v, %v", layer, err)
	}
	layers, err := client.ListFSLayers(context.Background())
	if err != nil || len(layers) != 1 {
		t.Fatalf("ListFSLayers = %#v, %v", layers, err)
	}
	if _, err := client.GetFSLayer(context.Background(), "layer-1"); err != nil {
		t.Fatalf("GetFSLayer: %v", err)
	}
	if entries, err := client.DiffFSLayerAtSeq(context.Background(), "layer-1", 7); err != nil || entries[0].EntrySeq != 7 {
		t.Fatalf("DiffFSLayerAtSeq = %#v, %v", entries, err)
	}
	if entries, err := client.ReplayFSLayer(context.Background(), "layer-1"); err != nil || entries[0].Path != "/repo/replay.txt" {
		t.Fatalf("ReplayFSLayer = %#v, %v", entries, err)
	}
	if _, err := client.UpsertFSLayerEntry(context.Background(), "layer-1", FSLayerEntryRequest{Path: "/repo/a.txt", Op: "upsert", Content: []byte("hello")}); err != nil {
		t.Fatalf("UpsertFSLayerEntry: %v", err)
	}
	if _, err := client.UploadFSLayerFile(context.Background(), "layer-1", "/repo/upload.txt", strings.NewReader("upload bytes"), int64(len("upload bytes")), 2, 0o640, true); err != nil {
		t.Fatalf("UploadFSLayerFile: %v", err)
	}
	if !sawUpload {
		t.Fatal("expected upload request")
	}
	maxSeq := int64(4)
	data, err := client.ReadFSLayerFile(context.Background(), "layer-1", "/repo/upload.txt", &maxSeq)
	if err != nil || string(data) != "upload bytes" {
		t.Fatalf("ReadFSLayerFile = %q, %v", data, err)
	}
	if _, err := client.GetFSLayerEntry(context.Background(), "layer-1", "/repo/a.txt"); err != nil {
		t.Fatalf("GetFSLayerEntry: %v", err)
	}
	if _, err := client.CheckpointFSLayer(context.Background(), "layer-1", FSLayerCheckpointRequest{CheckpointID: "cp-1"}); err != nil {
		t.Fatalf("CheckpointFSLayer: %v", err)
	}
	if _, err := client.GetFSLayerCheckpoint(context.Background(), "cp-1"); err != nil {
		t.Fatalf("GetFSLayerCheckpoint: %v", err)
	}
	if events, err := client.ListFSLayerEvents(context.Background(), "layer-1", 3); err != nil || len(events) != 1 {
		t.Fatalf("ListFSLayerEvents = %#v, %v", events, err)
	}
	if err := client.RollbackFSLayer(context.Background(), "layer-1"); err != nil {
		t.Fatalf("RollbackFSLayer: %v", err)
	}
	if commit, err := client.CommitFSLayer(context.Background(), "layer-1"); err != nil || commit.Status != "committed" {
		t.Fatalf("CommitFSLayer = %#v, %v", commit, err)
	}
}

func TestCommitFSLayerReturnsConflictBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/layers/layer-1/commit" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(FSLayerCommit{
			Status:  "conflicted",
			LayerID: "layer-1",
			Conflicts: []FSLayerCommitConflict{{
				Path:         "/repo/a.txt",
				Reason:       "base revision changed",
				BaseRevision: 3,
				WantRevision: 2,
			}},
		})
	}))
	defer server.Close()

	commit, err := testClient(t, server.URL).CommitFSLayer(context.Background(), "layer-1")
	if !errors.Is(err, ErrLayerCommitConflict) {
		t.Fatalf("CommitFSLayer err=%v, want ErrLayerCommitConflict", err)
	}
	if commit.Status != "conflicted" || len(commit.Conflicts) != 1 {
		t.Fatalf("commit=%+v, want conflict body", commit)
	}
	if commit.Conflicts[0].Path != "/repo/a.txt" || commit.Conflicts[0].WantRevision != 2 {
		t.Fatalf("conflict=%+v, want decoded conflict details", commit.Conflicts[0])
	}
}

func TestGrepWithLayerPassesLayerQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/fs/repo" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		if r.URL.Query().Get("grep") != "needle" || r.URL.Query().Get("layer") != "layer-1" {
			t.Fatalf("unexpected grep query: %s", r.URL.RawQuery)
		}
		_, _ = io.Copy(io.Discard, bytes.NewReader(nil))
		_ = json.NewEncoder(w).Encode([]SearchResult{{Path: "/repo/a.txt"}})
	}))
	defer server.Close()

	results, err := testClient(t, server.URL).GrepWithLayer(context.Background(), "/repo", "needle", 20, "layer-1")
	if err != nil {
		t.Fatalf("GrepWithLayer: %v", err)
	}
	if len(results) != 1 || results[0].Path != "/repo/a.txt" {
		t.Fatalf("results=%#v", results)
	}
}
