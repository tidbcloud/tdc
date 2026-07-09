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

	apifs "github.com/tidbcloud/tdc/internal/api/fs"
	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/config"
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
			if got := r.Header.Values("X-Dat9-Tag"); strings.Join(got, ",") != "owner=agent,topic=mvp" {
				t.Fatalf("X-Dat9-Tag = %v", got)
			}
			if got := r.Header.Get("X-Dat9-Description"); got != "readme upload" {
				t.Fatalf("X-Dat9-Description = %q", got)
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
		Profile:     profile,
		FromLocal:   localFile,
		ToRemote:    "/workspace/README.md",
		Overwrite:   false,
		ToLocal:     "",
		FromRemote:  "",
		Tags:        map[string]string{"topic": "mvp", "owner": "agent"},
		Description: "readme upload",
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

func TestChmodFilePersistsClientModeForDescribe(t *testing.T) {
	profile := dataProfile()
	var sawChmod bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/fs/workspace/README.md" && hasQueryKey(r.URL.RawQuery, "chmod"):
			sawChmod = true
			var body struct {
				Mode int64 `json:"mode"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode chmod body: %v", err)
			}
			if body.Mode != 0o600 {
				t.Fatalf("chmod mode = %#o", body.Mode)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace/README.md" && r.URL.Query().Get("stat") == "1":
			_ = json.NewEncoder(w).Encode(apifs.StatMetadataResponse{Size: 12, IsDir: false, Revision: 3, Mtime: 1700000000})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	service := testService(t.TempDir(), server.URL)
	chmod, err := service.ChmodFile(context.Background(), ChmodFileOptions{Profile: profile, Path: "/workspace/README.md", Mode: "0600"})
	if err != nil {
		t.Fatalf("ChmodFile failed: %v", err)
	}
	if !sawChmod || chmod.Status != "updated" {
		t.Fatalf("unexpected chmod result saw=%t result=%#v", sawChmod, chmod)
	}
	describe, err := service.DescribeFile(context.Background(), DescribeFileOptions{Profile: profile, Path: "/workspace/README.md"})
	if err != nil {
		t.Fatalf("DescribeFile failed: %v", err)
	}
	if !describe.HasMode || describe.Mode != 0o600 {
		t.Fatalf("describe mode = (%t,%#o), want (true,0600): %#v", describe.HasMode, describe.Mode, describe)
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
		case r.Method == http.MethodPost && r.URL.Path == "/v1/fs/workspace/README.md" && hasQueryKey(r.URL.RawQuery, "chmod"):
			var body struct {
				Mode int64 `json:"mode"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode chmod body: %v", err)
			}
			if body.Mode != 0o600 {
				t.Fatalf("chmod mode = %#o", body.Mode)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/fs/workspace/link.md" && r.URL.Query().Get("symlink") == "1":
			var body struct {
				Target string `json:"target"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode symlink body: %v", err)
			}
			if body.Target != "README.md" {
				t.Fatalf("symlink target = %q", body.Target)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/fs/workspace/hard.md" && r.URL.Query().Get("hardlink") == "1":
			if got := r.Header.Get("X-Dat9-Hardlink-Source"); got != "/workspace/README.md" {
				t.Fatalf("hardlink source = %q", got)
			}
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
	if _, err := service.ChmodFile(context.Background(), ChmodFileOptions{Profile: profile, Path: "/workspace/README.md", Mode: "0600"}); err != nil {
		t.Fatalf("ChmodFile failed: %v", err)
	}
	if _, err := service.SymlinkFile(context.Background(), SymlinkFileOptions{Profile: profile, Target: "README.md", Link: "/workspace/link.md"}); err != nil {
		t.Fatalf("SymlinkFile failed: %v", err)
	}
	if _, err := service.HardlinkFile(context.Background(), HardlinkFileOptions{Profile: profile, Source: "/workspace/README.md", Link: "/workspace/hard.md"}); err != nil {
		t.Fatalf("HardlinkFile failed: %v", err)
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

func TestReadFileRangeUsesRangeRequest(t *testing.T) {
	profile := dataProfile()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		if r.Method != http.MethodGet || r.URL.Path != "/v1/fs/workspace/README.md" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		if got := r.Header.Get("Range"); got != "bytes=6-8" {
			t.Fatalf("Range = %q", got)
		}
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("tdc"))
	}))
	defer server.Close()

	data, err := testService(t.TempDir(), server.URL).ReadFile(context.Background(), ReadFileOptions{
		Profile: profile,
		Path:    "/workspace/README.md",
		Range:   true,
		Offset:  6,
		Length:  3,
	})
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "tdc" {
		t.Fatalf("unexpected ranged data %q", data)
	}
}

func TestCopyFileAppendLocalToRemoteFallsBackToExpectedRevisionRewrite(t *testing.T) {
	profile := dataProfile()
	localFile := filepath.Join(t.TempDir(), "append.txt")
	if err := os.WriteFile(localFile, []byte(" world"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/v1/fs/workspace/README.md":
			w.Header().Set("Content-Length", "5")
			w.Header().Set("X-Dat9-Revision", "2")
			w.Header().Set("X-Dat9-IsDir", "false")
		case r.Method == http.MethodPost && r.URL.Path == "/v1/fs/workspace/README.md" && hasQueryKey(r.URL.RawQuery, "append"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("file is not S3-stored"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace/README.md":
			_, _ = w.Write([]byte("hello"))
		case r.Method == http.MethodPut && r.URL.Path == "/v1/fs/workspace/README.md":
			if got := r.Header.Get("X-Dat9-Expected-Revision"); got != "2" {
				t.Fatalf("X-Dat9-Expected-Revision = %q", got)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if string(body) != "hello world" {
				t.Fatalf("unexpected append body %q", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(apifs.WriteResponse{Revision: 3})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).CopyFile(context.Background(), CopyFileOptions{
		Profile:   profile,
		FromLocal: localFile,
		ToRemote:  "/workspace/README.md",
		Append:    true,
	})
	if err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}
	if result.Status != "appended" || result.BytesTransferred != int64(len(" world")) || result.Revision != 3 {
		t.Fatalf("unexpected append result: %#v", result)
	}
}

func TestCopyFileAppendLocalToRemoteUsesAppendPlan(t *testing.T) {
	profile := dataProfile()
	localFile := filepath.Join(t.TempDir(), "append.txt")
	if err := os.WriteFile(localFile, []byte("KLMNOP"), 0o644); err != nil {
		t.Fatal(err)
	}
	var uploaded bool
	var completed bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") && r.Header.Get("Authorization") != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/v1/fs/workspace/README.md":
			w.Header().Set("Content-Length", "10")
			w.Header().Set("X-Dat9-Revision", "9")
			w.Header().Set("X-Dat9-IsDir", "false")
		case r.Method == http.MethodPost && r.URL.Path == "/v1/fs/workspace/README.md" && hasQueryKey(r.URL.RawQuery, "append"):
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode append body: %v", err)
			}
			if body["append_size"] != float64(6) || body["expected_revision"] != float64(9) {
				t.Fatalf("unexpected append body: %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(apifs.AppendPlan{
				BaseSize: 10,
				PatchPlan: apifs.PatchPlan{
					UploadID: "append-1",
					PartSize: 8,
					UploadParts: []*apifs.PatchPartURL{{
						Number:      2,
						URL:         serverURL(r) + "/upload/2",
						Size:        8,
						Headers:     map[string]string{"X-Upload-Token": "append"},
						ReadURL:     serverURL(r) + "/read/2",
						ReadHeaders: map[string]string{"Range": "bytes=8-9"},
					}},
					CopiedParts: []int{1},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/read/2":
			if got := r.Header.Get("Range"); got != "bytes=8-9" {
				t.Fatalf("Range = %q", got)
			}
			_, _ = w.Write([]byte("ij"))
		case r.Method == http.MethodPut && r.URL.Path == "/upload/2":
			uploaded = true
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("presigned upload should not use tdc fs auth, got %q", got)
			}
			if got := r.Header.Get("X-Upload-Token"); got != "append" {
				t.Fatalf("X-Upload-Token = %q", got)
			}
			if got := r.Header.Get("x-amz-checksum-crc32c"); got != "" {
				t.Fatalf("append patch upload should not add unsigned crc32c checksum header, got %q", got)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read upload body: %v", err)
			}
			if string(body) != "ijKLMNOP" {
				t.Fatalf("unexpected append upload body %q", body)
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads/append-1/complete":
			completed = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).CopyFile(context.Background(), CopyFileOptions{
		Profile:   profile,
		FromLocal: localFile,
		ToRemote:  "/workspace/README.md",
		Append:    true,
	})
	if err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}
	if !uploaded || !completed || result.Status != "appended" || result.BytesTransferred != int64(len("KLMNOP")) || result.PartsUploaded != 1 {
		t.Fatalf("unexpected append result: %#v uploaded=%t completed=%t", result, uploaded, completed)
	}
}

func TestCopyFileResumeLocalToRemoteUsesActiveUpload(t *testing.T) {
	profile := dataProfile()
	payload := strings.Repeat("resume-payload-", 4096)
	localFile := filepath.Join(t.TempDir(), "resume.bin")
	if err := os.WriteFile(localFile, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	var uploaded bool
	var completed bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") && r.Header.Get("Authorization") != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/uploads":
			if r.URL.Query().Get("path") != "/workspace/resume.bin" || r.URL.Query().Get("status") != "UPLOADING" {
				t.Fatalf("unexpected upload query %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uploads": []apifs.UploadMeta{{UploadID: "upload-1", Path: "/workspace/resume.bin", PartsTotal: 1, Status: "UPLOADING"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads/upload-1/resume":
			var body struct {
				PartChecksums []string `json:"part_checksums"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode resume body: %v", err)
			}
			if len(body.PartChecksums) != 1 || body.PartChecksums[0] == "" {
				t.Fatalf("unexpected resume checksums: %#v", body.PartChecksums)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(apifs.UploadPlan{
				UploadID: "upload-1",
				PartSize: apifs.DefaultMultipartPartSize,
				Parts: []apifs.PartURL{{
					Number:  1,
					URL:     serverURL(r) + "/upload/1",
					Size:    int64(len(payload)),
					Headers: map[string]string{"X-Upload-Token": "resume"},
				}},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/upload/1":
			uploaded = true
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("presigned upload should not use tdc fs auth, got %q", got)
			}
			if got := r.Header.Get("X-Upload-Token"); got != "resume" {
				t.Fatalf("X-Upload-Token = %q", got)
			}
			if got := r.Header.Get("x-amz-checksum-crc32c"); got == "" {
				t.Fatalf("missing crc32c checksum header")
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read upload body: %v", err)
			}
			if string(body) != payload {
				t.Fatalf("unexpected upload body length %d", len(body))
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads/upload-1/complete":
			completed = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).CopyFile(context.Background(), CopyFileOptions{
		Profile:   profile,
		FromLocal: localFile,
		ToRemote:  "/workspace/resume.bin",
		Resume:    true,
	})
	if err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}
	if !uploaded || !completed || result.Status != "resumed" || result.PartsUploaded != 1 || result.BytesTransferred != int64(len(payload)) {
		t.Fatalf("unexpected resume result: %#v uploaded=%t completed=%t", result, uploaded, completed)
	}
}

func TestCopyFileResumeRemoteToLocalUsesRangeRequest(t *testing.T) {
	profile := dataProfile()
	target := filepath.Join(t.TempDir(), "download.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/v1/fs/workspace/README.md":
			w.Header().Set("Content-Length", "11")
			w.Header().Set("X-Dat9-IsDir", "false")
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace/README.md":
			if got := r.Header.Get("Range"); got != "bytes=5-10" {
				t.Fatalf("Range = %q", got)
			}
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte(" world"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).CopyFile(context.Background(), CopyFileOptions{
		Profile:    profile,
		FromRemote: "/workspace/README.md",
		ToLocal:    target,
		Resume:     true,
	})
	if err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read resumed file: %v", err)
	}
	if string(data) != "hello world" || result.Status != "resumed" || result.BytesTransferred != int64(len(" world")) {
		t.Fatalf("unexpected resume result %#v with data %q", result, data)
	}
}

func TestCopyFileRecursiveLocalToRemoteCopiesDirectoryContents(t *testing.T) {
	profile := dataProfile()
	sourceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(sourceRoot, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "nested", "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	var wroteA bool
	var wroteB bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodHead && (r.URL.Path == "/v1/fs/workspace/tree" || r.URL.Path == "/v1/fs/workspace/tree/a.txt" || r.URL.Path == "/v1/fs/workspace/tree/nested" || r.URL.Path == "/v1/fs/workspace/tree/nested/b.txt"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && (r.URL.Path == "/v1/fs/workspace/tree" || r.URL.Path == "/v1/fs/workspace/tree/nested") && hasQueryKey(r.URL.RawQuery, "mkdir"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/fs/workspace/tree/a.txt":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read a.txt body: %v", err)
			}
			if string(body) != "a" {
				t.Fatalf("unexpected a.txt body %q", body)
			}
			wroteA = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(apifs.WriteResponse{Revision: 1})
		case r.Method == http.MethodPut && r.URL.Path == "/v1/fs/workspace/tree/nested/b.txt":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read b.txt body: %v", err)
			}
			if string(body) != "b" {
				t.Fatalf("unexpected b.txt body %q", body)
			}
			wroteB = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(apifs.WriteResponse{Revision: 1})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).CopyFile(context.Background(), CopyFileOptions{
		Profile:   profile,
		FromLocal: sourceRoot,
		ToRemote:  "/workspace/tree",
		Recursive: true,
	})
	if err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}
	if !wroteA || !wroteB || result.FilesTransferred != 2 || result.BytesTransferred != 2 {
		t.Fatalf("unexpected recursive result: %#v wroteA=%t wroteB=%t", result, wroteA, wroteB)
	}
}

func TestCopyFileRecursiveLocalToRemoteRejectsSymlink(t *testing.T) {
	profile := dataProfile()
	sourceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, "target.txt"), []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.txt", filepath.Join(sourceRoot, "link.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/v1/fs/workspace/tree":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/fs/workspace/tree" && hasQueryKey(r.URL.RawQuery, "mkdir"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodHead && r.URL.Path == "/v1/fs/workspace/tree/target.txt":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/fs/workspace/tree/target.txt":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(apifs.WriteResponse{Revision: 1})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	_, err := testService(t.TempDir(), server.URL).CopyFile(context.Background(), CopyFileOptions{
		Profile:   profile,
		FromLocal: sourceRoot,
		ToRemote:  "/workspace/tree",
		Recursive: true,
	})
	if err == nil || !strings.Contains(err.Error(), "symlinks") {
		t.Fatalf("expected symlink rejection, got %v", err)
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

func serverURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
