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
	"github.com/Icemap/tdc/internal/apperr"
	"github.com/Icemap/tdc/internal/config"
)

func TestCopyFileLocalToRemoteUsesBearerAndRefusesOverwriteByDefault(t *testing.T) {
	profile := dataProfile()
	localFile := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(localFile, []byte("hello tdc fs"), 0o644); err != nil {
		t.Fatal(err)
	}
	var sawPut bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/v1/fs/workspace/README.md":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/fs/workspace/README.md":
			sawPut = true
			if got := r.Header.Get("Content-Type"); got != "application/octet-stream" {
				t.Fatalf("Content-Type = %q", got)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if string(body) != "hello tdc fs" {
				t.Fatalf("unexpected body %q", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(apifs.WriteResponse{Revision: 42})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).CopyFile(context.Background(), CopyFileOptions{
		Profile:    profile,
		FromLocal:  localFile,
		ToRemote:   "/workspace/README.md",
		Overwrite:  false,
		ToLocal:    "",
		FromRemote: "",
	})
	if err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}
	if !sawPut || result.Status != "copied" || result.Revision != 42 || result.BytesTransferred != int64(len("hello tdc fs")) {
		t.Fatalf("unexpected copy result: %#v", result)
	}

	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead || r.URL.Path != "/v1/fs/workspace/README.md" {
			t.Fatalf("unexpected overwrite guard request %s %s", r.Method, r.URL.RequestURI())
		}
		w.WriteHeader(http.StatusOK)
	})
	_, err = testService(t.TempDir(), server.URL).CopyFile(context.Background(), CopyFileOptions{
		Profile:   profile,
		FromLocal: localFile,
		ToRemote:  "/workspace/README.md",
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected target exists error, got %v", err)
	}
}

func TestCopyFileRemoteToLocalCreatesParents(t *testing.T) {
	profile := dataProfile()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		if r.Method != http.MethodGet || r.URL.Path != "/v1/fs/workspace/README.md" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		_, _ = w.Write([]byte("remote bytes"))
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "nested", "README.md")
	result, err := testService(t.TempDir(), server.URL).CopyFile(context.Background(), CopyFileOptions{
		Profile:       profile,
		FromRemote:    "/workspace/README.md",
		ToLocal:       target,
		CreateParents: true,
	})
	if err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(data) != "remote bytes" || result.BytesTransferred != int64(len("remote bytes")) {
		t.Fatalf("unexpected remote copy result %#v with data %q", result, data)
	}
}

func TestCopyFileRemoteToLocalUsesReadPermissionForAuthorizationErrors(t *testing.T) {
	profile := dataProfile()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/fs/workspace/README.md" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	_, err := testService(t.TempDir(), server.URL).CopyFile(context.Background(), CopyFileOptions{
		Profile:    profile,
		FromRemote: "/workspace/README.md",
		ToLocal:    filepath.Join(t.TempDir(), "README.md"),
	})
	if err == nil {
		t.Fatal("expected authorization error")
	}
	message := apperr.MessageFor(err)
	if !strings.Contains(message, "fs.file.read") || strings.Contains(message, "fs.file.write") {
		t.Fatalf("expected read permission error, got %q", message)
	}
}

func TestDataPlaneReadMetadataSearchAndMutations(t *testing.T) {
	profile := dataProfile()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace/README.md" && r.URL.RawQuery == "":
			_, _ = w.Write([]byte("file bytes"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace" && r.URL.Query().Get("list") == "1":
			_ = json.NewEncoder(w).Encode(apifs.ListResponse{Entries: []apifs.FileInfo{{Name: "README.md", Size: 10, IsDir: false, Mtime: 1700000000}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace/README.md" && r.URL.Query().Get("stat") == "1":
			_ = json.NewEncoder(w).Encode(apifs.StatMetadataResponse{Size: 10, IsDir: false, Revision: 3, ContentType: "text/plain"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/fs/workspace/copy.md" && hasQueryKey(r.URL.RawQuery, "copy"):
			if got := r.Header.Get("X-Dat9-Copy-Source"); got != "/workspace/README.md" {
				t.Fatalf("copy source = %q", got)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/fs/workspace/moved.md" && hasQueryKey(r.URL.RawQuery, "rename"):
			if got := r.Header.Get("X-Dat9-Rename-Source"); got != "/workspace/README.md" {
				t.Fatalf("rename source = %q", got)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/fs/workspace/old.md" && r.URL.Query().Get("recursive") == "1":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/fs/workspace/newdir" && hasQueryKey(r.URL.RawQuery, "mkdir") && r.URL.Query().Get("mode") == "700":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace" && r.URL.Query().Get("grep") == "hello":
			_ = json.NewEncoder(w).Encode([]apifs.SearchResult{{Path: "/workspace/README.md", Name: "README.md", SizeBytes: 10}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace" && hasQueryKey(r.URL.RawQuery, "find"):
			if r.URL.Query().Get("name") != "*.md" || r.URL.Query().Get("type") != "file" || r.URL.Query().Get("minsize") != "1" || r.URL.Query().Get("maxsize") != "100" {
				t.Fatalf("unexpected find query %q", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode([]apifs.SearchResult{{Path: "/workspace/README.md", Name: "README.md", SizeBytes: 10}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()
	service := testService(t.TempDir(), server.URL)

	data, err := service.ReadFile(context.Background(), ReadFileOptions{Profile: profile, Path: "/workspace/README.md"})
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "file bytes" {
		t.Fatalf("unexpected read data %q", data)
	}
	list, err := service.ListFiles(context.Background(), ListFilesOptions{Profile: profile, Path: "/workspace"})
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}
	if len(list.Entries) != 1 || list.Entries[0].Name != "README.md" {
		t.Fatalf("unexpected list result: %#v", list)
	}
	describe, err := service.DescribeFile(context.Background(), DescribeFileOptions{Profile: profile, Path: "/workspace/README.md"})
	if err != nil {
		t.Fatalf("DescribeFile failed: %v", err)
	}
	if describe.ContentType != "text/plain" || describe.Revision != 3 {
		t.Fatalf("unexpected describe result: %#v", describe)
	}
	if _, err := service.CopyFile(context.Background(), CopyFileOptions{Profile: profile, FromRemote: "/workspace/README.md", ToRemote: "/workspace/copy.md", Overwrite: true}); err != nil {
		t.Fatalf("remote CopyFile failed: %v", err)
	}
	if _, err := service.MoveFile(context.Background(), MoveFileOptions{Profile: profile, FromRemote: "/workspace/README.md", ToRemote: "/workspace/moved.md", Overwrite: true}); err != nil {
		t.Fatalf("MoveFile failed: %v", err)
	}
	if _, err := service.DeleteFile(context.Background(), DeleteFileOptions{Profile: profile, Path: "/workspace/old.md", Recursive: true}); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}
	if _, err := service.CreateDirectory(context.Background(), CreateDirectoryOptions{Profile: profile, Path: "/workspace/newdir", Mode: "0700"}); err != nil {
		t.Fatalf("CreateDirectory failed: %v", err)
	}
	search, err := service.SearchFileContent(context.Background(), SearchFileContentOptions{Profile: profile, Path: "/workspace", Pattern: "hello", Limit: 5})
	if err != nil {
		t.Fatalf("SearchFileContent failed: %v", err)
	}
	if len(search.Results) != 1 || search.Results[0].Path != "/workspace/README.md" {
		t.Fatalf("unexpected search result: %#v", search)
	}
	find, err := service.FindFiles(context.Background(), FindFilesOptions{Profile: profile, Path: "/workspace", FileNamePattern: "*.md", ResourceType: "file", MinSizeBytes: 1, MaxSizeBytes: 100})
	if err != nil {
		t.Fatalf("FindFiles failed: %v", err)
	}
	if len(find.Results) != 1 || find.Results[0].Name != "README.md" {
		t.Fatalf("unexpected find result: %#v", find)
	}
}

func TestReadFileRequiresFSAPIKey(t *testing.T) {
	_, err := testService(t.TempDir(), "https://fs.test").ReadFile(context.Background(), ReadFileOptions{Profile: testProfile(), Path: "/workspace/README.md"})
	if err == nil || !strings.Contains(err.Error(), "missing fs_api_key") {
		t.Fatalf("expected missing fs_api_key error, got %v", err)
	}
}

func dataProfile() *config.Profile {
	profile := testProfile()
	profile.FSResourceName = "workspace"
	profile.FSTenantID = "tenant-1"
	profile.FSCloudProvider = "aws"
	profile.FSRegionCode = "us-east-1"
	profile.FSAPIKey = "fs-secret"
	return profile
}

func hasQueryKey(rawQuery, key string) bool {
	for _, part := range strings.Split(rawQuery, "&") {
		if part == key || strings.HasPrefix(part, key+"=") {
			return true
		}
	}
	return false
}
