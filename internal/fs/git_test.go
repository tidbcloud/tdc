package fs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apifs "github.com/tidbcloud/tdc/internal/api/fs"
	"github.com/tidbcloud/tdc/internal/fs/mountstate"
)

func TestCreateGitWorkspaceSendsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/git-workspaces" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		var req apifs.GitWorkspaceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.RootPath != "/repo" || req.RepoURL != "https://example.test/repo.git" || req.RemoteName != "origin" || req.Mode != "fast" {
			t.Fatalf("request = %#v", req)
		}
		_ = json.NewEncoder(w).Encode(apifs.GitWorkspace{WorkspaceID: "gw-1", RootPath: req.RootPath, RepoURL: req.RepoURL, Mode: req.Mode})
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).CreateGitWorkspace(context.Background(), GitWorkspaceCreateOptions{
		Profile:    dataProfile(),
		RootPath:   "/repo",
		RepoURL:    "https://example.test/repo.git",
		RemoteName: "origin",
		Mode:       "fast",
	})
	if err != nil {
		t.Fatalf("CreateGitWorkspace failed: %v", err)
	}
	if result.WorkspaceID != "gw-1" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestReplaceGitTreeParsesNodeJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/git-workspaces/gw-1/tree" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		var req apifs.GitTreeReplaceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.CommitSHA != "abc123" || len(req.Nodes) != 1 || req.Nodes[0].Path != "README.md" || req.Nodes[0].SizeBytes != 5 {
			t.Fatalf("request = %#v", req)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).ReplaceGitTree(context.Background(), GitTreeReplaceOptions{
		Profile:     dataProfile(),
		WorkspaceID: "gw-1",
		CommitSHA:   "abc123",
		NodeJSON:    []string{`{"path":"README.md","parent_path":"","name":"README.md","kind":"file","mode":"100644","object_sha":"blob1","size_bytes":5}`},
	})
	if err != nil {
		t.Fatalf("ReplaceGitTree failed: %v", err)
	}
	if result.Status != "replaced" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGitStateAndOverlayRequests(t *testing.T) {
	var sawState bool
	var sawOverlay bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/git-workspaces/gw-1/git-state":
			sawState = true
			var req apifs.GitStateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode state request: %v", err)
			}
			if req.CheckpointCommit != "abc123" || req.StorageType != "inline" || string(req.Content) != "state" {
				t.Fatalf("state request = %#v", req)
			}
			_ = json.NewEncoder(w).Encode(apifs.GitState{WorkspaceID: "gw-1", CheckpointCommit: "abc123"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/git-workspaces/gw-1/overlay":
			sawOverlay = true
			var req apifs.GitOverlayEntryRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode overlay request: %v", err)
			}
			if req.Path != "README.md" || req.Op != "upsert" || req.Kind != "file" || string(req.Content) != "hello" {
				t.Fatalf("overlay request = %#v", req)
			}
			_ = json.NewEncoder(w).Encode(apifs.GitOverlayEntry{WorkspaceID: "gw-1", Path: "README.md", Op: "upsert"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()
	service := testService(t.TempDir(), server.URL)

	if _, err := service.UpsertGitState(context.Background(), GitStateUpsertOptions{Profile: dataProfile(), WorkspaceID: "gw-1", CheckpointCommit: "abc123", StorageType: "inline", Content: "state"}); err != nil {
		t.Fatalf("UpsertGitState failed: %v", err)
	}
	if _, err := service.PutGitOverlayEntry(context.Background(), GitOverlayPutOptions{Profile: dataProfile(), WorkspaceID: "gw-1", Path: "README.md", Operation: "upsert", ResourceKind: "file", Content: "hello"}); err != nil {
		t.Fatalf("PutGitOverlayEntry failed: %v", err)
	}
	if !sawState || !sawOverlay {
		t.Fatalf("expected state and overlay requests")
	}
}

func TestReplaceGitTreeRequiresNodeJSON(t *testing.T) {
	_, err := testService(t.TempDir(), "https://fs.test").ReplaceGitTree(context.Background(), GitTreeReplaceOptions{Profile: dataProfile(), WorkspaceID: "gw-1"})
	if err == nil || !strings.Contains(err.Error(), "node-json") {
		t.Fatalf("expected node-json error, got %v", err)
	}
}

func TestCloneGitWorkspaceRegistersTreeAndState(t *testing.T) {
	requireGitBinary(t)
	ctx := context.Background()
	sourceRepo := createTestGitRepo(t)
	home := t.TempDir()
	mountPath := filepath.Join(home, "mnt")
	if err := os.MkdirAll(mountPath, 0o755); err != nil {
		t.Fatalf("mkdir mount path: %v", err)
	}

	var sawWorkspace bool
	var sawTree bool
	var sawState bool
	var sawObjectPack bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/git-workspaces":
			sawWorkspace = true
			var req apifs.GitWorkspaceRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode workspace request: %v", err)
			}
			if req.RootPath != "/workspace/repo" || req.RemoteName != "origin" || req.Mode != "fast" || req.WorkspaceKind != "main" {
				t.Fatalf("workspace request = %#v", req)
			}
			_ = json.NewEncoder(w).Encode(apifs.GitWorkspace{
				WorkspaceID:   "gw-1",
				RootPath:      req.RootPath,
				RepoURL:       req.RepoURL,
				RemoteName:    req.RemoteName,
				BranchName:    req.BranchName,
				HeadCommit:    req.HeadCommit,
				Mode:          req.Mode,
				WorkspaceKind: req.WorkspaceKind,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/git-workspaces/gw-1/tree":
			sawTree = true
			var req apifs.GitTreeReplaceRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode tree request: %v", err)
			}
			if req.CommitSHA == "" || len(req.Nodes) == 0 || req.Nodes[0].Path != "README.md" {
				t.Fatalf("tree request = %#v", req)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/git-workspaces/gw-1/object-packs":
			sawObjectPack = true
			var req apifs.GitObjectPackRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode object pack request: %v", err)
			}
			if len(req.Content) == 0 {
				t.Fatal("object pack request has no content")
			}
			_ = json.NewEncoder(w).Encode(apifs.GitObjectPack{WorkspaceID: "gw-1", PackID: "pack-1", SizeBytes: int64(len(req.Content))})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/git-workspaces/gw-1/git-state":
			sawState = true
			var req apifs.GitStateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode state request: %v", err)
			}
			if req.StorageType != gitStateStorageTarGzNoObjects || req.SizeBytes == 0 || len(req.Content) == 0 {
				t.Fatalf("state request = %#v", req)
			}
			_ = json.NewEncoder(w).Encode(apifs.GitState{WorkspaceID: "gw-1", CheckpointCommit: req.CheckpointCommit})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	state, err := mountstate.New("stage", "workspace", mountPath, "/workspace", "fuse", server.URL, os.Getpid(), false, time.Now())
	if err != nil {
		t.Fatalf("mount state: %v", err)
	}
	state.LocalRoot = filepath.Join(home, "local")
	state.MountProfile = defaultMountProfile
	if _, err := mountstate.Write(home, state); err != nil {
		t.Fatalf("write mount state: %v", err)
	}

	target := filepath.Join(mountPath, "repo")
	result, err := testService(home, server.URL).CloneGitWorkspace(ctx, GitWorkspaceCloneOptions{
		Profile:    dataProfile(),
		RepoURL:    sourceRepo,
		TargetPath: target,
	})
	if err != nil {
		t.Fatalf("CloneGitWorkspace failed: %v", err)
	}
	if result.Workspace.WorkspaceID != "gw-1" || result.RemotePath != "/workspace/repo" || result.TreeEntries == 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !sawWorkspace || !sawTree || !sawState || !sawObjectPack {
		t.Fatalf("expected workspace/tree/state/object-pack requests, got workspace=%v tree=%v state=%v object_pack=%v", sawWorkspace, sawTree, sawState, sawObjectPack)
	}
}

func TestRestoreGitWorkspaceRestoresStateAndObjectPacks(t *testing.T) {
	requireGitBinary(t)
	ctx := context.Background()
	sourceRepo := createTestGitRepo(t)
	head := gitTestOutput(t, sourceRepo, "rev-parse", "HEAD")
	blob := gitTestOutput(t, sourceRepo, "rev-parse", "HEAD:README.md")
	gitDir := filepath.Join(sourceRepo, ".git")
	stateContent, err := archiveGitStateDir(gitDir)
	if err != nil {
		t.Fatalf("archive git state: %v", err)
	}
	packContent, err := packReachableGitObjects(ctx, gitDir)
	if err != nil {
		t.Fatalf("pack git objects: %v", err)
	}
	if len(packContent) == 0 {
		t.Fatal("expected object pack content")
	}

	home := t.TempDir()
	mountPath := filepath.Join(home, "mnt")
	localRoot := filepath.Join(home, "local")
	if err := os.MkdirAll(mountPath, 0o755); err != nil {
		t.Fatalf("mkdir mount path: %v", err)
	}
	state, err := mountstate.New("stage", "workspace", mountPath, "/workspace", "fuse", "https://fs.test", os.Getpid(), false, time.Now())
	if err != nil {
		t.Fatalf("mount state: %v", err)
	}
	state.LocalRoot = localRoot
	state.MountProfile = defaultMountProfile
	if _, err := mountstate.Write(home, state); err != nil {
		t.Fatalf("write mount state: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces" && r.URL.Query().Get("root_path") == "/workspace/repo":
			_ = json.NewEncoder(w).Encode(apifs.GitWorkspace{
				WorkspaceID:   "gw-1",
				RootPath:      "/workspace/repo",
				RepoURL:       "https://example.test/repo.git",
				HeadCommit:    head,
				Mode:          "fast",
				WorkspaceKind: "main",
				Status:        "active",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/git-state":
			_ = json.NewEncoder(w).Encode(apifs.GitState{
				WorkspaceID:      "gw-1",
				CheckpointCommit: head,
				StorageType:      gitStateStorageTarGzNoObjects,
				SizeBytes:        int64(len(stateContent)),
				Content:          stateContent,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/object-packs":
			_ = json.NewEncoder(w).Encode(apifs.GitObjectPacksResponse{Packs: []apifs.GitObjectPack{{WorkspaceID: "gw-1", PackID: "pack-1", SizeBytes: int64(len(packContent))}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/object-packs/pack-1":
			_ = json.NewEncoder(w).Encode(apifs.GitObjectPack{WorkspaceID: "gw-1", PackID: "pack-1", SizeBytes: int64(len(packContent)), Content: packContent})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()
	state.Endpoint = server.URL
	if _, err := mountstate.Write(home, state); err != nil {
		t.Fatalf("rewrite mount state: %v", err)
	}

	target := filepath.Join(mountPath, "repo")
	result, err := testService(home, server.URL).RestoreGitWorkspace(ctx, GitWorkspaceRestoreOptions{Profile: dataProfile(), TargetPath: target})
	if err != nil {
		t.Fatalf("RestoreGitWorkspace failed: %v", err)
	}
	if result.Status != "restored" || result.ObjectPacks != 1 || !result.StateRestored {
		t.Fatalf("unexpected restore result: %#v", result)
	}
	got := gitTestOutput(t, filepath.Dir(result.GitDir), "cat-file", "-p", blob)
	if got != "hello from git" {
		t.Fatalf("restored blob = %q", got)
	}
}

func requireGitBinary(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

func createTestGitRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runTestGit(t, "", "init", repo)
	runTestGit(t, repo, "config", "user.email", "tdc@example.test")
	runTestGit(t, repo, "config", "user.name", "tdc test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello from git\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runTestGit(t, repo, "add", "README.md")
	runTestGit(t, repo, "commit", "-m", "initial")
	return repo
}

func runTestGit(t *testing.T, repoDir string, args ...string) {
	t.Helper()
	if repoDir != "" {
		args = append([]string{"-C", repoDir}, args...)
	}
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
