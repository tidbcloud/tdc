//go:build !windows

package fs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	apifs "github.com/Icemap/tdc/internal/api/fs"
	"github.com/Icemap/tdc/internal/authz"
	"github.com/Icemap/tdc/internal/fs/mountcontrol"
	gofs "github.com/hanwen/go-fuse/v2/fs"
	gofuse "github.com/hanwen/go-fuse/v2/fuse"
)

func TestRemoteFuseRuntimeRetargetsOpenHandles(t *testing.T) {
	runtime := &remoteFuseRuntime{openHandles: map[*remoteFuseFile]struct{}{}}
	handle := newRemoteFuseFile(runtime, "/workspace/old/child.txt", []byte("alpha"), true, true, fuseObjectVersion{Revision: 1})

	runtime.retargetOpenHandles("/workspace/old", "/workspace/new")

	if handle.remotePath != "/workspace/new/child.txt" {
		t.Fatalf("unexpected remote path %q", handle.remotePath)
	}
}

func TestRemoteFuseRuntimeMarksDeletedOpenHandles(t *testing.T) {
	runtime := &remoteFuseRuntime{openHandles: map[*remoteFuseFile]struct{}{}}
	handle := newRemoteFuseFile(runtime, "/workspace/old/child.txt", []byte("alpha"), true, true, fuseObjectVersion{Revision: 1})

	runtime.markDeletedOpenHandles("/workspace/old")

	if !handle.deleted {
		t.Fatal("expected descendant handle to be marked deleted")
	}
}

func TestRemoteFuseRuntimeLocalOverlayOpenAndWrite(t *testing.T) {
	localRoot := t.TempDir()
	runtime := &remoteFuseRuntime{
		remoteRoot: "/workspace",
		localRoot:  localRoot,
		profile:    defaultMountProfile,
		readCache:  newFuseReadCache(0, 0, 0),
	}
	if !runtime.isLocalOnly("/workspace/repo/.git/config") {
		t.Fatal(".git path should be local-only")
	}
	if runtime.isLocalOnly("/workspace/repo/.gitignore") {
		t.Fatal(".gitignore should stay remote-persistent")
	}
	if runtime.localParentExists("/workspace/local-write.txt") {
		t.Fatal("root-level ordinary writes should not use the local overlay in coding-agent profile")
	}
	if runtime.localParentExists("/workspace/repo/ordinary.txt") {
		t.Fatal("ordinary writes under non-local-only parents should stay remote-persistent in coding-agent profile")
	}

	localFile := filepath.Join(localRoot, "overlay", "repo", "node_modules", "pkg.txt")
	if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
		t.Fatalf("mkdir local overlay: %v", err)
	}
	if err := os.WriteFile(localFile, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("write local overlay seed: %v", err)
	}
	if !runtime.localParentExists("/workspace/repo/node_modules/next.txt") {
		t.Fatal("children below local-only directories should use the local overlay")
	}
	fileNode := &remoteFuseNode{runtime: runtime, remotePath: "/workspace/repo/node_modules/pkg.txt"}
	handle, _, errno := fileNode.Open(context.Background(), uint32(os.O_RDWR|os.O_TRUNC))
	if errno != gofs.OK {
		t.Fatalf("Open errno = %v", errno)
	}
	writer, ok := handle.(gofs.FileWriter)
	if !ok {
		t.Fatalf("local open handle does not implement FileWriter: %T", handle)
	}
	if _, errno := writer.Write(context.Background(), []byte("local overlay\n"), 0); errno != gofs.OK {
		t.Fatalf("Write errno = %v", errno)
	}
	if releaser, ok := handle.(gofs.FileReleaser); ok {
		if errno := releaser.Release(context.Background()); errno != gofs.OK {
			t.Fatalf("Release errno = %v", errno)
		}
	}
	got, err := os.ReadFile(localFile)
	if err != nil {
		t.Fatalf("read local overlay file: %v", err)
	}
	if string(got) != "local overlay\n" {
		t.Fatalf("local overlay file = %q", got)
	}
}

func TestRemoteFuseRuntimeReadsAndWritesGitWorkspace(t *testing.T) {
	requireGitBinary(t)
	localRoot := t.TempDir()
	repoRoot := filepath.Join(localRoot, "overlay", "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}
	runTestGit(t, "", "init", repoRoot)
	runTestGit(t, repoRoot, "config", "user.email", "tdc@example.test")
	runTestGit(t, repoRoot, "config", "user.name", "tdc test")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("clean git content\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runTestGit(t, repoRoot, "add", "README.md")
	runTestGit(t, repoRoot, "commit", "-m", "initial")
	head := gitTestOutput(t, repoRoot, "rev-parse", "HEAD")
	blob := gitTestOutput(t, repoRoot, "rev-parse", "HEAD:README.md")
	if err := os.Remove(filepath.Join(repoRoot, "README.md")); err != nil {
		t.Fatalf("remove checked-out README: %v", err)
	}

	var sawOverlay bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces":
			_ = json.NewEncoder(w).Encode(apifs.GitWorkspacesResponse{Workspaces: []apifs.GitWorkspace{{
				WorkspaceID:   "gw-1",
				RootPath:      "/workspace/repo",
				RepoURL:       "https://example.test/repo.git",
				HeadCommit:    head,
				Mode:          "fast",
				WorkspaceKind: "main",
				Status:        "active",
			}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/tree":
			if r.URL.Query().Get("commit_sha") != head {
				t.Fatalf("commit_sha = %q", r.URL.Query().Get("commit_sha"))
			}
			_ = json.NewEncoder(w).Encode(apifs.GitTreeResponse{Nodes: []apifs.GitTreeNode{{
				Path:       "README.md",
				ParentPath: "",
				Name:       "README.md",
				Kind:       "file",
				Mode:       "100644",
				ObjectSHA:  blob,
				SizeBytes:  int64(len("clean git content\n")),
			}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/overlay":
			_ = json.NewEncoder(w).Encode(apifs.GitOverlayEntriesResponse{Entries: []apifs.GitOverlayEntry{}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/git-workspaces/gw-1/overlay":
			sawOverlay = true
			var req apifs.GitOverlayEntryRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode overlay request: %v", err)
			}
			if req.Path != "README.md" || req.Op != "upsert" || req.Kind != "file" || string(req.Content) != "dirty\n" {
				t.Fatalf("overlay request = %#v", req)
			}
			_ = json.NewEncoder(w).Encode(apifs.GitOverlayEntry{WorkspaceID: "gw-1", Path: req.Path, Op: req.Op, Kind: req.Kind})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	service := testService(t.TempDir(), server.URL)
	client, err := service.dataClient(dataProfile(), authz.FSGitWorkspaceWrite, "git fuse test")
	if err != nil {
		t.Fatalf("create data client: %v", err)
	}
	runtime := &remoteFuseRuntime{
		client:      client,
		mountPath:   filepath.Join(t.TempDir(), "mount"),
		remoteRoot:  "/workspace",
		localRoot:   localRoot,
		profile:     defaultMountProfile,
		git:         newFuseGitRuntime(),
		readCache:   newFuseReadCache(0, 0, 0),
		openHandles: map[*remoteFuseFile]struct{}{},
	}
	node := &remoteFuseNode{runtime: runtime, remotePath: "/workspace/repo/README.md"}
	handle, _, errno := node.Open(context.Background(), uint32(os.O_RDONLY))
	if errno != gofs.OK {
		t.Fatalf("Open read errno = %v", errno)
	}
	reader := handle.(gofs.FileReader)
	result, errno := reader.Read(context.Background(), make([]byte, 1024), 0)
	if errno != gofs.OK {
		t.Fatalf("Read errno = %v", errno)
	}
	data, status := result.Bytes(nil)
	if status != gofuse.OK {
		t.Fatalf("ReadResult status = %v", status)
	}
	if string(data) != "clean git content\n" {
		t.Fatalf("read data = %q", data)
	}
	if releaser, ok := handle.(gofs.FileReleaser); ok {
		if errno := releaser.Release(context.Background()); errno != gofs.OK {
			t.Fatalf("Release read errno = %v", errno)
		}
	}

	handle, _, errno = node.Open(context.Background(), uint32(os.O_RDWR|os.O_TRUNC))
	if errno != gofs.OK {
		t.Fatalf("Open write errno = %v", errno)
	}
	writer := handle.(gofs.FileWriter)
	if _, errno := writer.Write(context.Background(), []byte("dirty\n"), 0); errno != gofs.OK {
		t.Fatalf("Write errno = %v", errno)
	}
	if errno := handle.(gofs.FileReleaser).Release(context.Background()); errno != gofs.OK {
		t.Fatalf("Release write errno = %v", errno)
	}
	if !sawOverlay {
		t.Fatal("expected git overlay write")
	}
}

func TestRemoteFuseRuntimeRestoresGitStateForCleanRead(t *testing.T) {
	requireGitBinary(t)
	ctx := context.Background()
	sourceRepo := createTestGitRepo(t)
	head := gitTestOutput(t, sourceRepo, "rev-parse", "HEAD")
	blob := gitTestOutput(t, sourceRepo, "rev-parse", "HEAD:README.md")
	stateContent, err := archiveGitStateDir(filepath.Join(sourceRepo, ".git"))
	if err != nil {
		t.Fatalf("archive git state: %v", err)
	}
	packContent, err := packReachableGitObjects(ctx, filepath.Join(sourceRepo, ".git"))
	if err != nil {
		t.Fatalf("pack git objects: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/git-state":
			_ = json.NewEncoder(w).Encode(apifs.GitState{WorkspaceID: "gw-1", CheckpointCommit: head, StorageType: gitStateStorageTarGzNoObjects, Content: stateContent, SizeBytes: int64(len(stateContent))})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/object-packs":
			_ = json.NewEncoder(w).Encode(apifs.GitObjectPacksResponse{Packs: []apifs.GitObjectPack{{WorkspaceID: "gw-1", PackID: "pack-1", SizeBytes: int64(len(packContent))}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/object-packs/pack-1":
			_ = json.NewEncoder(w).Encode(apifs.GitObjectPack{WorkspaceID: "gw-1", PackID: "pack-1", Content: packContent, SizeBytes: int64(len(packContent))})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()
	service := testService(t.TempDir(), server.URL)
	client, err := service.dataClient(dataProfile(), authz.FSGitWorkspaceRead, "git restore test")
	if err != nil {
		t.Fatalf("create data client: %v", err)
	}
	runtime := &remoteFuseRuntime{
		client:     client,
		mountPath:  filepath.Join(t.TempDir(), "mount"),
		remoteRoot: "/workspace",
		localRoot:  t.TempDir(),
		profile:    defaultMountProfile,
		git:        newFuseGitRuntime(),
		readCache:  newFuseReadCache(0, 0, 0),
	}
	entry := fuseGitEntry{
		workspace: apifs.GitWorkspace{WorkspaceID: "gw-1", RootPath: "/workspace/repo", HeadCommit: head, WorkspaceKind: "main", Status: "active"},
		relPath:   "README.md",
		clean:     &apifs.GitTreeNode{Path: "README.md", Kind: "file", Mode: "100644", ObjectSHA: blob, SizeBytes: int64(len("hello from git\n"))},
	}
	data, err := runtime.gitReadFile(ctx, entry)
	if err != nil {
		t.Fatalf("gitReadFile failed: %v", err)
	}
	if string(data) != "hello from git\n" {
		t.Fatalf("gitReadFile data = %q", data)
	}
	if _, err := os.Stat(filepath.Join(runtime.localRoot, "overlay", "repo", ".git")); err != nil {
		t.Fatalf("expected restored .git directory: %v", err)
	}
}

func TestRemoteFuseSetattrModeCallsRemoteChmod(t *testing.T) {
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
	client, err := service.dataClient(dataProfile(), authz.FSFileWrite, "chmod tdc fs file")
	if err != nil {
		t.Fatalf("create data client: %v", err)
	}
	node := &remoteFuseNode{runtime: &remoteFuseRuntime{client: client, readCache: newFuseReadCache(0, 0, 0)}, remotePath: "/workspace/README.md"}
	var out gofuse.AttrOut
	errno := node.Setattr(context.Background(), nil, &gofuse.SetAttrIn{
		SetAttrInCommon: gofuse.SetAttrInCommon{
			Valid: gofuse.FATTR_MODE,
			Mode:  0o600,
		},
	}, &out)
	if errno != gofs.OK {
		t.Fatalf("Setattr errno = %v", errno)
	}
	if !sawChmod {
		t.Fatal("expected remote chmod request")
	}
}

func TestRemoteFuseReadlinkUsesClientMetadata(t *testing.T) {
	store, err := newFSMetadataStore(t.TempDir(), dataProfile())
	if err != nil {
		t.Fatalf("metadata store: %v", err)
	}
	if err := store.setSymlink("/workspace/link.txt", "README.md"); err != nil {
		t.Fatalf("set symlink metadata: %v", err)
	}
	node := &remoteFuseNode{
		runtime:    &remoteFuseRuntime{metadata: store, readCache: newFuseReadCache(0, 0, 0)},
		remotePath: "/workspace/link.txt",
	}
	target, errno := node.Readlink(context.Background())
	if errno != gofs.OK {
		t.Fatalf("Readlink errno = %v", errno)
	}
	if string(target) != "README.md" {
		t.Fatalf("Readlink target = %q", target)
	}
}

func gitTestOutput(t *testing.T, repoDir string, args ...string) string {
	t.Helper()
	args = append([]string{"-C", repoDir}, args...)
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func TestFuseDirtyRangesMergeAndPartSelection(t *testing.T) {
	ranges := mergeFuseDirtyRanges([]fuseDirtyRange{
		{Start: 10, End: 20},
		{Start: 0, End: 5},
		{Start: 4, End: 12},
	})
	if len(ranges) != 1 || ranges[0].Start != 0 || ranges[0].End != 20 {
		t.Fatalf("unexpected merged ranges: %#v", ranges)
	}
	parts := fuseDirtyParts([]fuseDirtyRange{{Start: 9, End: 21}}, 10, 100)
	if len(parts) != 3 || parts[0] != 1 || parts[1] != 2 || parts[2] != 3 {
		t.Fatalf("unexpected dirty parts: %#v", parts)
	}
}

func TestShouldPatchFuseWrite(t *testing.T) {
	version := fuseObjectVersion{Revision: 7, ResourceID: "file-a"}
	if !shouldPatchFuseWrite(version, 100, 100, []fuseDirtyRange{{Start: 10, End: 20}}) {
		t.Fatal("expected small same-size dirty write to use patch")
	}
	if shouldPatchFuseWrite(version, 100, 120, []fuseDirtyRange{{Start: 10, End: 20}}) {
		t.Fatal("size-changing write should use whole upload")
	}
	if shouldPatchFuseWrite(fuseObjectVersion{}, 100, 100, []fuseDirtyRange{{Start: 10, End: 20}}) {
		t.Fatal("unknown base version should use whole upload")
	}
	if shouldPatchFuseWrite(version, 100, 100, []fuseDirtyRange{{Start: 0, End: 100}}) {
		t.Fatal("full-file dirty write should use whole upload")
	}
}

func TestRemoteFuseRuntimeUploadUsesPatchForDirtyRange(t *testing.T) {
	var sawPatch bool
	var sawWholePut bool
	var sawPatchUpload bool
	headCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") && r.Header.Get("Authorization") != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace/large.bin" && r.URL.Query().Get("stat") == "1":
			headCalls++
			_ = json.NewEncoder(w).Encode(apifs.StatMetadataResponse{
				Size:       12,
				IsDir:      false,
				Revision:   int64(6 + headCalls),
				ResourceID: "resource-1",
			})
		case r.Method == http.MethodHead && r.URL.Path == "/v1/fs/workspace/large.bin":
			headCalls++
			w.Header().Set("Content-Length", "12")
			w.Header().Set("X-Dat9-Revision", strconv.Itoa(6+headCalls))
			w.Header().Set("X-Dat9-Resource-Id", "resource-1")
			w.Header().Set("X-Dat9-IsDir", "false")
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/fs/workspace/large.bin":
			sawPatch = true
			var body struct {
				NewSize          int64  `json:"new_size"`
				DirtyParts       []int  `json:"dirty_parts"`
				PartSize         int64  `json:"part_size"`
				ExpectedRevision *int64 `json:"expected_revision,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			if body.NewSize != 12 || len(body.DirtyParts) != 1 || body.DirtyParts[0] != 1 || body.ExpectedRevision == nil || *body.ExpectedRevision != 7 {
				t.Fatalf("unexpected patch body: %#v", body)
			}
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(apifs.PatchPlan{
				UploadID: "patch-1",
				PartSize: body.PartSize,
				UploadParts: []*apifs.PatchPartURL{{
					Number: 1,
					URL:    serverURL(r) + "/patch/1",
					Size:   12,
				}},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/patch/1":
			sawPatchUpload = true
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("presigned upload should not receive tdc auth, got %q", got)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read patch upload body: %v", err)
			}
			if string(body) != "hello_patch!" {
				t.Fatalf("unexpected patch upload body %q", body)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads/patch-1/complete":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/fs/workspace/large.bin":
			sawWholePut = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(apifs.WriteResponse{Revision: 9})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	service := testService(t.TempDir(), server.URL)
	client, err := service.dataClient(dataProfile(), authz.FSFileWrite, "write tdc fs file")
	if err != nil {
		t.Fatalf("create data client: %v", err)
	}
	runtime := &remoteFuseRuntime{client: client, readCache: newFuseReadCache(0, 0, 0), openHandles: map[*remoteFuseFile]struct{}{}}
	version, err := runtime.upload(context.Background(), "/workspace/large.bin", []byte("hello_patch!"), fuseObjectVersion{Revision: 7, ResourceID: "resource-1"}, 12, []fuseDirtyRange{{Start: 6, End: 11}})
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if !sawPatch || !sawPatchUpload || sawWholePut {
		t.Fatalf("expected patch upload only, sawPatch=%t sawPatchUpload=%t sawWholePut=%t", sawPatch, sawPatchUpload, sawWholePut)
	}
	if version.Revision != 8 {
		t.Fatalf("expected refreshed revision 8, got %#v", version)
	}
}

func TestDrainAllowsCleanOpenHandles(t *testing.T) {
	runtime := &remoteFuseRuntime{
		mountPath:   "/mnt/tdc",
		readCache:   newFuseReadCache(0, 0, 0),
		openHandles: map[*remoteFuseFile]struct{}{},
	}
	newRemoteFuseFile(runtime, "/workspace/clean.txt", []byte("clean"), false, false, fuseObjectVersion{Revision: 1})

	resp := runtime.Drain(context.Background())
	if !resp.OK {
		t.Fatalf("Drain OK = false, error_kind=%q error=%q pending=%+v", resp.ErrorKind, resp.Error, resp.Pending)
	}
	if resp.Pending.OpenHandles != 1 || resp.Pending.DirtyHandles != 0 {
		t.Fatalf("pending = %+v, want one clean open handle", resp.Pending)
	}
}

func TestDrainFlushesDirtyOpenHandles(t *testing.T) {
	var wroteBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/fs/workspace/dirty.txt" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		wroteBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apifs.WriteResponse{Revision: 2})
	}))
	defer server.Close()

	service := testService(t.TempDir(), server.URL)
	client, err := service.dataClient(dataProfile(), authz.FSFileWrite, "drain dirty FUSE handle")
	if err != nil {
		t.Fatalf("create data client: %v", err)
	}
	runtime := &remoteFuseRuntime{
		mountPath:   "/mnt/tdc",
		client:      client,
		readCache:   newFuseReadCache(0, 0, 0),
		openHandles: map[*remoteFuseFile]struct{}{},
	}
	handle := newRemoteFuseFile(runtime, "/workspace/dirty.txt", []byte("dirty"), true, true, fuseObjectVersion{})

	resp := runtime.Drain(context.Background())
	if !resp.OK {
		t.Fatalf("Drain OK = false, error_kind=%q error=%q pending=%+v", resp.ErrorKind, resp.Error, resp.Pending)
	}
	if wroteBody != "dirty" {
		t.Fatalf("uploaded body = %q, want dirty", wroteBody)
	}
	if handle.dirty {
		t.Fatal("dirty handle remained dirty after drain")
	}
	if resp.Pending.OpenHandles != 1 || resp.Pending.DirtyHandles != 0 {
		t.Fatalf("pending = %+v, want clean open handle", resp.Pending)
	}
}

func TestDrainFailsWhenFinalDirtyHandleRemains(t *testing.T) {
	runtime := &remoteFuseRuntime{
		mountPath:   "/mnt/tdc",
		readCache:   newFuseReadCache(0, 0, 0),
		openHandles: map[*remoteFuseFile]struct{}{},
	}
	newRemoteFuseFile(runtime, "/workspace/dirty.txt", []byte("dirty"), true, true, fuseObjectVersion{})

	resp := runtime.Drain(context.Background())
	if resp.OK {
		t.Fatalf("Drain OK = true with pending=%+v", resp.Pending)
	}
	if resp.ErrorKind == "" {
		t.Fatalf("ErrorKind empty, response=%+v", resp)
	}
	if resp.Pending.DirtyHandles != 1 {
		t.Fatalf("DirtyHandles = %d, want 1", resp.Pending.DirtyHandles)
	}
	if !hasDrainPhase(resp, "flush_open_handles") {
		t.Fatalf("phases = %+v, want flush_open_handles phase", resp.Phases)
	}
}

func TestDrainFailsWhenUploaderCacheRemains(t *testing.T) {
	store := newFuseWriteBackStore(t.TempDir(), 1<<20, testMountCacheIdentity())
	if err := store.put("/workspace/cached.txt", []byte("data"), fuseObjectVersion{}, 0, nil); err != nil {
		t.Fatalf("put pending write: %v", err)
	}
	runtime := &remoteFuseRuntime{
		mountPath:   "/mnt/tdc",
		writeBack:   store,
		readCache:   newFuseReadCache(0, 0, 0),
		openHandles: map[*remoteFuseFile]struct{}{},
	}

	resp := runtime.Drain(context.Background())
	if resp.OK {
		t.Fatalf("Drain OK = true with pending=%+v", resp.Pending)
	}
	if resp.Pending.UploaderCached != 1 || resp.Pending.UploaderCachedBytes != 4 {
		t.Fatalf("uploader cached pending = %+v, want one 4-byte cache entry", resp.Pending)
	}
	if resp.ErrorKind == "" {
		t.Fatalf("ErrorKind empty, response=%+v", resp)
	}
}

func TestMountControlServerHandlesDrainRequest(t *testing.T) {
	runtime := &remoteFuseRuntime{
		mountPath:   filepath.Join(t.TempDir(), "mount"),
		readCache:   newFuseReadCache(0, 0, 0),
		openHandles: map[*remoteFuseFile]struct{}{},
	}
	if err := os.MkdirAll(runtime.mountPath, 0o755); err != nil {
		t.Fatal(err)
	}
	server, err := startMountControlServer(runtime.mountPath, runtime)
	if err != nil {
		t.Fatalf("start control server: %v", err)
	}
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := mountcontrol.RequestDrain(ctx, server.SocketPath(), time.Second)
	if err != nil {
		t.Fatalf("RequestDrain failed: %v", err)
	}
	if !resp.OK || resp.MountPoint != runtime.mountPath {
		t.Fatalf("unexpected drain response: %#v", resp)
	}
}

func hasDrainPhase(resp mountcontrol.DrainResponse, name string) bool {
	for _, phase := range resp.Phases {
		if phase.Name == name {
			return true
		}
	}
	return false
}
