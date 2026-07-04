package fs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitWorkspaceClientMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/git-workspaces":
			var req GitWorkspaceRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode workspace request: %v", err)
			}
			if req.RootPath != "/repo" || req.RepoURL != "https://example.test/repo.git" || req.Mode != "fast" {
				t.Fatalf("workspace request = %#v", req)
			}
			_ = json.NewEncoder(w).Encode(GitWorkspace{WorkspaceID: "gw-1", RootPath: req.RootPath, RepoURL: req.RepoURL, Mode: req.Mode})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces" && r.URL.RawQuery == "":
			_ = json.NewEncoder(w).Encode(GitWorkspacesResponse{Workspaces: []GitWorkspace{{WorkspaceID: "gw-1", RootPath: "/repo"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces" && r.URL.Query().Get("root_path") == "/repo":
			_ = json.NewEncoder(w).Encode(GitWorkspace{WorkspaceID: "gw-1", RootPath: "/repo"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1":
			_ = json.NewEncoder(w).Encode(GitWorkspace{WorkspaceID: "gw-1", RootPath: "/repo"})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/git-workspaces/gw-1":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/git-workspaces/gw-1/tree":
			var req GitTreeReplaceRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode tree request: %v", err)
			}
			if req.CommitSHA != "abc123" || len(req.Nodes) != 1 || req.Nodes[0].Path != "README.md" {
				t.Fatalf("tree request = %#v", req)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/tree":
			if r.URL.Query().Get("commit_sha") != "abc123" {
				t.Fatalf("tree query = %q", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(GitTreeResponse{Nodes: []GitTreeNode{{Path: "README.md", Kind: "file"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/git-workspaces/gw-1/git-state":
			var req GitStateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode git state request: %v", err)
			}
			if req.CheckpointCommit != "abc123" || string(req.Content) != "state" {
				t.Fatalf("git state request = %#v", req)
			}
			_ = json.NewEncoder(w).Encode(GitState{WorkspaceID: "gw-1", CheckpointCommit: "abc123", Content: []byte("state")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/git-state":
			_ = json.NewEncoder(w).Encode(GitState{WorkspaceID: "gw-1", CheckpointCommit: "abc123"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/git-workspaces/gw-1/object-packs":
			var req GitObjectPackRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode object pack request: %v", err)
			}
			if string(req.Content) != "pack" {
				t.Fatalf("object pack content = %q", req.Content)
			}
			_ = json.NewEncoder(w).Encode(GitObjectPack{WorkspaceID: "gw-1", PackID: "pack-1", Content: []byte("pack")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/object-packs":
			_ = json.NewEncoder(w).Encode(GitObjectPacksResponse{Packs: []GitObjectPack{{WorkspaceID: "gw-1", PackID: "pack-1"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/object-packs/pack-1":
			_ = json.NewEncoder(w).Encode(GitObjectPack{WorkspaceID: "gw-1", PackID: "pack-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/git-workspaces/gw-1/overlay":
			var req GitOverlayEntryRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode overlay request: %v", err)
			}
			if req.Path != "README.md" || req.Op != "upsert" || string(req.Content) != "hello" {
				t.Fatalf("overlay request = %#v", req)
			}
			_ = json.NewEncoder(w).Encode(GitOverlayEntry{WorkspaceID: "gw-1", Path: "README.md", Op: "upsert", Content: []byte("hello")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/overlay" && r.URL.Query().Get("path") == "README.md":
			_ = json.NewEncoder(w).Encode(GitOverlayEntry{WorkspaceID: "gw-1", Path: "README.md"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/git-workspaces/gw-1/overlay" && r.URL.RawQuery == "":
			_ = json.NewEncoder(w).Encode(GitOverlayEntriesResponse{Entries: []GitOverlayEntry{{WorkspaceID: "gw-1", Path: "README.md"}}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	client := testBearerClient(t, server.URL)
	ctx := context.Background()
	workspace, err := client.UpsertGitWorkspace(ctx, GitWorkspaceRequest{RootPath: "/repo", RepoURL: "https://example.test/repo.git", Mode: "fast"})
	if err != nil || workspace.WorkspaceID != "gw-1" {
		t.Fatalf("UpsertGitWorkspace = %#v, %v", workspace, err)
	}
	if listed, err := client.ListGitWorkspaces(ctx); err != nil || len(listed.Workspaces) != 1 {
		t.Fatalf("ListGitWorkspaces = %#v, %v", listed, err)
	}
	if byRoot, err := client.GetGitWorkspaceByRoot(ctx, "/repo"); err != nil || byRoot.WorkspaceID != "gw-1" {
		t.Fatalf("GetGitWorkspaceByRoot = %#v, %v", byRoot, err)
	}
	if byID, err := client.GetGitWorkspace(ctx, "gw-1"); err != nil || byID.WorkspaceID != "gw-1" {
		t.Fatalf("GetGitWorkspace = %#v, %v", byID, err)
	}
	if err := client.ReplaceGitTree(ctx, "gw-1", GitTreeReplaceRequest{CommitSHA: "abc123", Nodes: []GitTreeNode{{Path: "README.md", Name: "README.md", Kind: "file"}}}); err != nil {
		t.Fatalf("ReplaceGitTree: %v", err)
	}
	if tree, err := client.ListGitTree(ctx, "gw-1", "abc123"); err != nil || len(tree.Nodes) != 1 {
		t.Fatalf("ListGitTree = %#v, %v", tree, err)
	}
	if state, err := client.UpsertGitState(ctx, "gw-1", GitStateRequest{CheckpointCommit: "abc123", Content: []byte("state")}); err != nil || state.WorkspaceID != "gw-1" {
		t.Fatalf("UpsertGitState = %#v, %v", state, err)
	}
	if state, err := client.GetGitState(ctx, "gw-1"); err != nil || state.WorkspaceID != "gw-1" {
		t.Fatalf("GetGitState = %#v, %v", state, err)
	}
	if pack, err := client.PutGitObjectPack(ctx, "gw-1", GitObjectPackRequest{Content: []byte("pack")}); err != nil || pack.PackID != "pack-1" {
		t.Fatalf("PutGitObjectPack = %#v, %v", pack, err)
	}
	if packs, err := client.ListGitObjectPacks(ctx, "gw-1"); err != nil || len(packs.Packs) != 1 {
		t.Fatalf("ListGitObjectPacks = %#v, %v", packs, err)
	}
	if pack, err := client.GetGitObjectPack(ctx, "gw-1", "pack-1"); err != nil || pack.PackID != "pack-1" {
		t.Fatalf("GetGitObjectPack = %#v, %v", pack, err)
	}
	if overlay, err := client.PutGitOverlayEntry(ctx, "gw-1", GitOverlayEntryRequest{Path: "README.md", Op: "upsert", Content: []byte("hello")}); err != nil || overlay.Path != "README.md" {
		t.Fatalf("PutGitOverlayEntry = %#v, %v", overlay, err)
	}
	if overlay, err := client.GetGitOverlayEntry(ctx, "gw-1", "README.md"); err != nil || overlay.Path != "README.md" {
		t.Fatalf("GetGitOverlayEntry = %#v, %v", overlay, err)
	}
	if overlays, err := client.ListGitOverlayEntries(ctx, "gw-1"); err != nil || len(overlays.Entries) != 1 {
		t.Fatalf("ListGitOverlayEntries = %#v, %v", overlays, err)
	}
	if err := client.DeleteGitWorkspace(ctx, "gw-1"); err != nil {
		t.Fatalf("DeleteGitWorkspace: %v", err)
	}
}
