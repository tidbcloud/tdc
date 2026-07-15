package fs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	apifs "github.com/tidbcloud/tdc/internal/api/fs"
	"github.com/tidbcloud/tdc/internal/fs/mountstate"
)

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
