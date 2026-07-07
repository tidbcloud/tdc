package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Icemap/tdc/internal/api"
	"github.com/Icemap/tdc/internal/api/endpoints"
	apifs "github.com/Icemap/tdc/internal/api/fs"
	"github.com/Icemap/tdc/internal/auth"
	"github.com/Icemap/tdc/internal/authz"
	"github.com/Icemap/tdc/internal/config"
)

const defaultLiveProfile = "live-e2e"

func TestLiveProfileConfigured(t *testing.T) {
	requireLive(t)

	bin := tdcBinary(t)
	version := runTDC(t, bin, "--version")
	version.wantExitCode(0)
	version.wantStdoutContains("tdc ")

	profile := liveProfile(t)
	if profile.CloudProvider == "" || profile.RegionCode == "" {
		t.Fatalf("live e2e profile %q is incomplete", profile.Name)
	}
}

func TestLiveTiDBCloudAPIReadOnlyProbes(t *testing.T) {
	requireLive(t)

	profile := liveProfile(t)
	resolver := endpoints.NewResolver()

	starterEndpoint, err := resolver.ResolveStarter(profile.CloudProvider, profile.RegionCode)
	if err != nil {
		t.Fatalf("resolve Starter endpoint: %v", err)
	}
	starter := liveDigestClient(t, profile, starterEndpoint, authz.StarterClusterRead)
	liveGETJSON(t, starter, "/v1beta1/regions")
	liveGETJSON(t, starter, "/v1beta1/regions:listCloudProviders")

	iamEndpoint, err := resolver.ResolveIAM()
	if err != nil {
		t.Fatalf("resolve IAM endpoint: %v", err)
	}
	iam := liveDigestClient(t, profile, iamEndpoint, authz.OrganizationProjectRead)
	liveGETJSON(t, iam, "/v1beta1/projects")
}

func TestLiveCurrentCommandSurface(t *testing.T) {
	requireLive(t)

	bin := tdcBinary(t)
	profileName := liveProfileName(t)

	helpCommands := [][]string{
		{"help"},
		{"cli", "help"},
		{"db", "help"},
		{"fs", "help"},
		{"git", "help"},
		{"journal", "help"},
		{"vault", "help"},
		{"organization", "help"},
		{"db", "create-db-cluster", "help"},
		{"db", "list-db-clusters", "help"},
		{"fs", "create-file-system", "help"},
		{"fs", "copy-file", "help"},
		{"fs", "read-file", "help"},
		{"fs", "chmod-file", "help"},
		{"fs", "create-symlink", "help"},
		{"fs", "create-hardlink", "help"},
		{"fs", "create-layer", "help"},
		{"fs", "list-layers", "help"},
		{"fs", "describe-layer", "help"},
		{"fs", "diff-layer", "help"},
		{"fs", "replay-layer", "help"},
		{"fs", "create-layer-entry", "help"},
		{"fs", "upload-layer-file", "help"},
		{"fs", "read-layer-file", "help"},
		{"fs", "describe-layer-entry", "help"},
		{"fs", "create-layer-checkpoint", "help"},
		{"fs", "describe-layer-checkpoint", "help"},
		{"fs", "list-layer-events", "help"},
		{"fs", "rollback-layer", "help"},
		{"fs", "commit-layer", "help"},
		{"fs", "pack-file-system", "help"},
		{"fs", "unpack-file-system", "help"},
		{"fs", "drain-file-system", "help"},
		{"fs", "cp", "help"},
		{"fs", "cat", "help"},
		{"fs", "ls", "help"},
		{"fs", "stat", "help"},
		{"fs", "mv", "help"},
		{"fs", "rm", "help"},
		{"fs", "mkdir", "help"},
		{"fs", "chmod", "help"},
		{"fs", "symlink", "help"},
		{"fs", "hardlink", "help"},
		{"fs", "grep", "help"},
		{"fs", "find", "help"},
		{"fs", "mount", "help"},
		{"fs", "drain", "help"},
		{"fs", "umount", "help"},
		{"vault", "create-secret", "help"},
		{"vault", "replace-secret", "help"},
		{"vault", "read-secret", "help"},
		{"vault", "list-secrets", "help"},
		{"vault", "delete-secret", "help"},
		{"vault", "create-token", "help"},
		{"vault", "delete-token", "help"},
		{"vault", "create-grant", "help"},
		{"vault", "delete-grant", "help"},
		{"vault", "list-audit-events", "help"},
		{"vault", "run-with-secret", "help"},
		{"vault", "mount-vault", "help"},
		{"vault", "unmount-vault", "help"},
		{"journal", "create-journal", "help"},
		{"journal", "append-journal-entries", "help"},
		{"journal", "read-journal-entries", "help"},
		{"journal", "search-journal-entries", "help"},
		{"journal", "verify-journal", "help"},
		{"git", "clone-git-workspace", "help"},
		{"git", "hydrate-git-workspace", "help"},
		{"git", "add-git-worktree", "help"},
		{"git", "remove-git-worktree", "help"},
		{"git", "create-git-workspace", "help"},
		{"git", "list-git-workspaces", "help"},
		{"git", "describe-git-workspace", "help"},
		{"git", "delete-git-workspace", "help"},
		{"git", "replace-git-tree", "help"},
		{"git", "list-git-tree", "help"},
		{"git", "upsert-git-state", "help"},
		{"git", "describe-git-state", "help"},
		{"git", "put-git-object-pack", "help"},
		{"git", "list-git-object-packs", "help"},
		{"git", "describe-git-object-pack", "help"},
		{"git", "put-git-overlay-entry", "help"},
		{"git", "describe-git-overlay-entry", "help"},
		{"git", "list-git-overlay-entries", "help"},
	}
	for _, args := range helpCommands {
		result := runTDC(t, bin, args...)
		result.wantExitCode(0)
		result.wantStdoutContains("Usage:")
	}

	mutatingDryRunCommands := [][]string{
		{"db", "create-db-cluster-branch", "--db-cluster-id", "cluster-1", "--db-cluster-branch-name", "dev"},
		{"db", "delete-db-cluster-branch", "--db-cluster-id", "cluster-1", "--db-cluster-branch-id", "branch-1", "--confirm-db-cluster-branch-name", "dev"},
		{"db", "prepare-db-query-access", "--db-cluster-id", "cluster-1"},
		{"fs", "create-file-system", "--file-system-name", "workspace"},
		{"fs", "delete-file-system", "--file-system-name", "workspace", "--confirm-file-system-name", "workspace"},
		{"fs", "create-layer", "--layer-id", "layer-1", "--base-root-path", "/workspace", "--layer-name", "dev"},
		{"fs", "create-layer-entry", "--layer-id", "layer-1", "--path", "/workspace/a.txt", "--content", "hello"},
		{"fs", "upload-layer-file", "--layer-id", "layer-1", "--from-local", "/tmp/tdc-e2e-local.txt", "--to-layer-path", "/workspace/a.txt"},
		{"fs", "create-layer-checkpoint", "--layer-id", "layer-1", "--checkpoint-id", "cp-1"},
		{"fs", "rollback-layer", "--layer-id", "layer-1"},
		{"fs", "commit-layer", "--layer-id", "layer-1"},
		{"fs", "pack-file-system", "--local-root", "/tmp/tdc-e2e-pack", "--remote-root", "/workspace", "--mount-profile", "portable"},
		{"fs", "unpack-file-system", "--local-root", "/tmp/tdc-e2e-pack", "--remote-root", "/workspace", "--mount-profile", "portable"},
		{"fs", "mount-file-system", "--mount-path", "/tmp/tdc-e2e-mount", "--driver", "webdav"},
		{"fs", "unmount-file-system", "--mount-path", "/tmp/tdc-e2e-mount"},
		{"vault", "create-secret", "--secret-name", "tdc-e2e-secret", "--field", "DB_URL=mysql://example"},
		{"vault", "replace-secret", "--secret-path", "/n/vault/tdc-e2e-secret", "--from-directory", "/tmp"},
		{"vault", "delete-secret", "--secret-name", "tdc-e2e-secret"},
		{"vault", "create-token", "--agent-id", "tdc-live-e2e", "--task-id", "task-1", "--scope", "tdc-e2e-secret", "--ttl", "10m"},
		{"vault", "delete-token", "--token-id", "token-1"},
		{"vault", "create-grant", "--agent-id", "tdc-live-e2e", "--scope", "tdc-e2e-secret/DB_URL", "--permission", "read", "--ttl", "10m"},
		{"vault", "delete-grant", "--grant-id", "grant-1"},
		{"vault", "mount-vault", "--mount-path", "/tmp/tdc-e2e-vault"},
		{"vault", "unmount-vault", "--mount-path", "/tmp/tdc-e2e-vault"},
		{"journal", "create-journal", "--journal-id", "jrn-tdc-e2e", "--journal-kind", "agent"},
		{"journal", "append-journal-entries", "--journal-id", "jrn-tdc-e2e", "--entry-json", `{"type":"task.started"}`},
		{"git", "create-git-workspace", "--root-path", "/repo", "--repo-url", "https://example.test/repo.git"},
		{"git", "delete-git-workspace", "--workspace-id", "gw-1"},
		{"git", "replace-git-tree", "--workspace-id", "gw-1", "--commit-sha", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--node-json", `{"path":"README.md","name":"README.md","kind":"file"}`},
		{"git", "upsert-git-state", "--workspace-id", "gw-1", "--checkpoint-commit", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"git", "put-git-object-pack", "--workspace-id", "gw-1", "--content", "pack"},
		{"git", "put-git-overlay-entry", "--workspace-id", "gw-1", "--path", "README.md", "--operation", "upsert"},
	}
	for _, args := range mutatingDryRunCommands {
		fullArgs := append([]string{"--profile", profileName}, args...)
		fullArgs = append(fullArgs, "--dry-run", "--query", "checks[].name")
		result := runTDC(t, bin, fullArgs...)
		result.wantExitCode(0)
		result.wantStdoutContains("config_and_credentials")
		result.wantStdoutContains("endpoint_selection")
		result.wantStdoutContains("permission_requirement")
		result.wantStdoutContains("remote_mutation")
	}

	dataPlaneDryRunCommands := [][]string{
		{"fs", "copy-file", "--from-remote", "/workspace/source.txt", "--to-remote", "/workspace/target.txt"},
		{"fs", "move-file", "--from-remote", "/workspace/source.txt", "--to-remote", "/workspace/target.txt"},
		{"fs", "delete-file", "--path", "/workspace/source.txt"},
		{"fs", "create-directory", "--path", "/workspace/newdir"},
		{"fs", "chmod-file", "--path", "/workspace/source.txt", "--mode", "0600"},
		{"fs", "create-symlink", "--target", "source.txt", "--link-path", "/workspace/link.txt"},
		{"fs", "create-hardlink", "--source-path", "/workspace/source.txt", "--link-path", "/workspace/hard.txt"},
	}
	for _, args := range dataPlaneDryRunCommands {
		fullArgs := append([]string{"--profile", profileName}, args...)
		fullArgs = append(fullArgs, "--dry-run", "--query", "checks[].name")
		result := runTDC(t, bin, fullArgs...)
		result.wantExitCode(0)
		result.wantStdoutContains("config_and_credentials")
		result.wantStdoutContains("endpoint_selection")
		result.wantStdoutContains("permission_requirement")
		result.wantStdoutContains("request_construction")
	}

	readOnlyCommands := [][]string{
		{"organization", "list-projects"},
		{"db", "list-db-clusters"},
		{"db", "describe-db-cluster"},
		{"db", "list-db-cluster-branches"},
		{"db", "describe-db-cluster-branch"},
		{"db", "create-db-connection-string"},
		{"db", "execute-sql-statement"},
		{"fs", "check-file-system"},
		{"fs", "read-file"},
		{"fs", "list-files"},
		{"fs", "describe-file"},
		{"fs", "search-file-content"},
		{"fs", "find-files"},
		{"fs", "list-layers"},
		{"fs", "describe-layer"},
		{"fs", "diff-layer"},
		{"fs", "replay-layer"},
		{"fs", "read-layer-file"},
		{"fs", "describe-layer-entry"},
		{"fs", "describe-layer-checkpoint"},
		{"fs", "list-layer-events"},
		{"vault", "read-secret"},
		{"vault", "list-secrets"},
		{"vault", "list-audit-events"},
		{"vault", "run-with-secret"},
		{"journal", "read-journal-entries"},
		{"journal", "search-journal-entries"},
		{"journal", "verify-journal"},
		{"git", "list-git-workspaces"},
		{"git", "describe-git-workspace"},
		{"git", "list-git-tree"},
		{"git", "describe-git-state"},
		{"git", "list-git-object-packs"},
		{"git", "describe-git-object-pack"},
		{"git", "describe-git-overlay-entry"},
		{"git", "list-git-overlay-entries"},
	}
	for _, args := range readOnlyCommands {
		fullArgs := append([]string{"--profile", profileName}, args...)
		fullArgs = append(fullArgs, "--dry-run")
		result := runTDC(t, bin, fullArgs...)
		result.wantExitCode(2)
		result.wantStderrContains("unknown flag: --dry-run")
	}

	projects := runTDC(t, bin, "--profile", profileName, "organization", "list-projects", "--page-size", "1")
	projects.wantExitCode(0)
	projects.wantStdoutContains(`"projects"`)
	var projectList struct {
		Projects []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"projects"`
		NextPageToken string `json:"next_page_token"`
	}
	if err := json.Unmarshal([]byte(projects.stdout), &projectList); err != nil {
		t.Fatalf("decode organization list-projects output: %v\n%s", err, projects.stdout)
	}
	if len(projectList.Projects) == 0 || projectList.Projects[0].ID == "" {
		t.Fatalf("expected live profile %q to see at least one project with an id:\n%s", profileName, projects.stdout)
	}

	query := runTDC(t, bin, "--profile", profileName, "organization", "list-projects", "--page-size", "1", "--query", "projects[0].id")
	query.wantExitCode(0)
	query.wantStdoutContains(projectList.Projects[0].ID)

	human := runTDC(t, bin, "--profile", profileName, "organization", "list-projects", "--page-size", "1", "--output", "human")
	human.wantExitCode(0)
	human.wantStdoutContains("ID")
	human.wantStdoutContains(projectList.Projects[0].ID)

	clusters := runTDC(t, bin, "--profile", profileName, "db", "list-db-clusters", "--page-size", "1")
	clusters.wantExitCode(0)
	clusters.wantStdoutContains(`"clusters"`)
	var clusterList struct {
		Clusters []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal([]byte(clusters.stdout), &clusterList); err != nil {
		t.Fatalf("decode db list-db-clusters output: %v\n%s", err, clusters.stdout)
	}

	clusterQuery := runTDC(t, bin, "--profile", profileName, "db", "list-db-clusters", "--page-size", "1", "--query", "clusters[].id")
	clusterQuery.wantExitCode(0)

	clusterHuman := runTDC(t, bin, "--profile", profileName, "db", "list-db-clusters", "--page-size", "1", "--output", "human")
	clusterHuman.wantExitCode(0)
	clusterHuman.wantStdoutContains("ID")

	if len(clusterList.Clusters) > 0 && clusterList.Clusters[0].ID != "" {
		describe := runTDC(t, bin, "--profile", profileName, "db", "describe-db-cluster", "--db-cluster-id", clusterList.Clusters[0].ID)
		describe.wantExitCode(0)
		describe.wantStdoutContains(`"id"`)
		describe.wantStdoutContains(clusterList.Clusters[0].ID)
	}

	checkUpdateHelp := runTDC(t, bin, "cli", "check-update", "help")
	checkUpdateHelp.wantExitCode(0)
	checkUpdateHelp.wantStdoutContains("--fail-if-update-available")

	updateHelp := runTDC(t, bin, "cli", "update", "help")
	updateHelp.wantExitCode(0)
	updateHelp.wantStdoutContains("--yes")
}

func TestLiveFSDataPlaneLifecycle(t *testing.T) {
	requireLive(t)

	profile := liveProfile(t)
	if profile.FSAPIKey == "" {
		t.Fatalf("live profile %q has no fs_api_key; run tdc fs create-file-system --profile %s before make live-e2e", profile.Name, profile.Name)
	}

	bin := tdcBinary(t)
	profileName := liveProfileName(t)
	suffix := time.Now().UTC().Format("20060102150405")
	rootPath := "/tdc-e2e-" + suffix
	sourcePath := rootPath + "/README.md"
	copyPath := rootPath + "/README.copy.md"
	movedPath := rootPath + "/README.moved.md"
	deleted := false
	defer func() {
		if deleted {
			return
		}
		cleanup := runTDC(t, bin, "--profile", profileName, "fs", "delete-file", "--path", rootPath, "--recursive")
		if cleanup.exitCode != 0 && cleanup.exitCode != 5 {
			t.Logf("cleanup delete failed for %s: exit=%d stdout=%s stderr=%s", rootPath, cleanup.exitCode, cleanup.stdout, cleanup.stderr)
		}
	}()

	createDir := runTDC(t, bin, "--profile", profileName, "fs", "create-directory", "--path", rootPath, "--mode", "0755")
	createDir.wantExitCode(0)
	createDir.wantStdoutContains(`"status": "created"`)

	content := "hello tdc fs live e2e " + suffix + "\n"
	localFile := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(localFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write local test file: %v", err)
	}

	upload := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-local", localFile, "--to-remote", sourcePath)
	upload.wantExitCode(0)
	upload.wantStdoutContains(`"status": "copied"`)
	upload.wantStdoutContains(`"bytes_transferred"`)

	list := runTDC(t, bin, "--profile", profileName, "fs", "list-files", "--path", rootPath)
	list.wantExitCode(0)
	list.wantStdoutContains("README.md")

	listHuman := runTDC(t, bin, "--profile", profileName, "fs", "list-files", "--path", rootPath, "--output", "human")
	listHuman.wantExitCode(0)
	listHuman.wantStdoutContains("NAME")
	listHuman.wantStdoutContains("README.md")

	describe := runTDC(t, bin, "--profile", profileName, "fs", "describe-file", "--path", sourcePath)
	describe.wantExitCode(0)
	describe.wantStdoutContains(`"size_bytes"`)

	read := runTDC(t, bin, "--profile", profileName, "fs", "read-file", "--path", sourcePath)
	read.wantExitCode(0)
	if read.stdout != content {
		read.fail("read-file should return raw file bytes exactly")
	}

	rangeRead := runTDC(t, bin, "--profile", profileName, "fs", "read-file", "--path", sourcePath, "--offset", "6", "--length", "3")
	rangeRead.wantExitCode(0)
	if rangeRead.stdout != "tdc" {
		rangeRead.fail("read-file --offset/--length should return the requested byte range")
	}

	appendText := "appended live e2e " + suffix + "\n"
	appendFile := filepath.Join(t.TempDir(), "append.txt")
	if err := os.WriteFile(appendFile, []byte(appendText), 0o644); err != nil {
		t.Fatalf("write append file: %v", err)
	}
	appendRemote := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-local", appendFile, "--to-remote", sourcePath, "--append")
	appendRemote.wantExitCode(0)
	appendRemote.wantStdoutContains(`"status": "appended"`)
	fullContent := content + appendText
	readAppended := runTDC(t, bin, "--profile", profileName, "fs", "read-file", "--path", sourcePath)
	readAppended.wantExitCode(0)
	if readAppended.stdout != fullContent {
		readAppended.fail("read-file should include appended bytes")
	}

	stdinPath := rootPath + "/stdin.txt"
	stdinContent := "stdin live e2e " + suffix + "\n"
	stdinUpload := runTDCWithInput(t, bin, stdinContent, nil, "--profile", profileName, "fs", "copy-file", "--from-stdin", "--to-remote", stdinPath, "--tag", "source=stdin", "--tag", "suite=live-e2e", "--description", "tdc live e2e stdin")
	stdinUpload.wantExitCode(0)
	stdinUpload.wantStdoutContains(`"status": "copied"`)
	stdinDownload := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-remote", stdinPath, "--to-stdout")
	stdinDownload.wantExitCode(0)
	if stdinDownload.stdout != stdinContent {
		stdinDownload.fail("copy-file --to-stdout should return raw file bytes exactly")
	}

	taggedPath := rootPath + "/tagged.md"
	taggedUpload := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-local", localFile, "--to-remote", taggedPath, "--tag", "source=local", "--tag", "suite=live-e2e", "--description", "tdc live e2e tagged")
	taggedUpload.wantExitCode(0)
	taggedUpload.wantStdoutContains(`"status": "copied"`)
	taggedDescribe := runTDC(t, bin, "--profile", profileName, "fs", "describe-file", "--path", taggedPath)
	taggedDescribe.wantExitCode(0)
	taggedDescribe.wantStdoutContains(`"source": "local"`)
	taggedDescribe.wantStdoutContains(`"suite": "live-e2e"`)

	chmod := runTDC(t, bin, "--profile", profileName, "fs", "chmod-file", "--path", sourcePath, "--mode", "0600")
	chmod.wantExitCode(0)
	chmod.wantStdoutContains(`"status": "updated"`)
	describeMode := runTDC(t, bin, "--profile", profileName, "fs", "describe-file", "--path", sourcePath)
	describeMode.wantExitCode(0)
	describeMode.wantStdoutContains(sourcePath)

	symlinkPath := rootPath + "/README.link.md"
	symlink := runTDC(t, bin, "--profile", profileName, "fs", "create-symlink", "--target", "README.md", "--link-path", symlinkPath)
	symlink.wantExitCode(0)
	symlink.wantStdoutContains(`"status": "created"`)
	listSymlink := runTDC(t, bin, "--profile", profileName, "fs", "list-files", "--path", rootPath)
	listSymlink.wantExitCode(0)
	listSymlink.wantStdoutContains("README.link.md")

	hardlinkPath := rootPath + "/README.hard.md"
	hardlink := runTDC(t, bin, "--profile", profileName, "fs", "create-hardlink", "--source-path", sourcePath, "--link-path", hardlinkPath)
	hardlink.wantExitCode(0)
	hardlink.wantStdoutContains(`"status": "created"`)
	readHardlink := runTDC(t, bin, "--profile", profileName, "fs", "read-file", "--path", hardlinkPath)
	readHardlink.wantExitCode(0)
	if readHardlink.stdout != fullContent {
		readHardlink.fail("hardlink should read the source file contents")
	}

	aliasDir := rootPath + "/alias"
	aliasMkdir := runTDC(t, bin, "--profile", profileName, "fs", "mkdir", "--path", aliasDir, "--mode", "0755")
	aliasMkdir.wantExitCode(0)
	aliasMkdir.wantStdoutContains(`"status": "created"`)

	aliasContent := "alias live e2e " + suffix + "\n"
	aliasLocalFile := filepath.Join(t.TempDir(), "alias.txt")
	if err := os.WriteFile(aliasLocalFile, []byte(aliasContent), 0o644); err != nil {
		t.Fatalf("write alias local file: %v", err)
	}
	aliasPath := aliasDir + "/alias.txt"
	aliasUpload := runTDC(t, bin, "--profile", profileName, "fs", "cp", "--from-local", aliasLocalFile, "--to-remote", aliasPath)
	aliasUpload.wantExitCode(0)
	aliasUpload.wantStdoutContains(`"status": "copied"`)

	aliasList := runTDC(t, bin, "--profile", profileName, "fs", "ls", "--path", aliasDir)
	aliasList.wantExitCode(0)
	aliasList.wantStdoutContains("alias.txt")

	aliasStat := runTDC(t, bin, "--profile", profileName, "fs", "stat", "--path", aliasPath)
	aliasStat.wantExitCode(0)
	aliasStat.wantStdoutContains(`"size_bytes"`)

	aliasRead := runTDC(t, bin, "--profile", profileName, "fs", "cat", "--path", aliasPath)
	aliasRead.wantExitCode(0)
	if aliasRead.stdout != aliasContent {
		aliasRead.fail("cat alias should return raw file bytes exactly")
	}

	aliasChmod := runTDC(t, bin, "--profile", profileName, "fs", "chmod", "--path", aliasPath, "--mode", "0600")
	aliasChmod.wantExitCode(0)
	aliasChmod.wantStdoutContains(`"status": "updated"`)

	aliasSymlinkPath := aliasDir + "/alias.link"
	aliasSymlink := runTDC(t, bin, "--profile", profileName, "fs", "symlink", "--target", "alias.txt", "--link-path", aliasSymlinkPath)
	aliasSymlink.wantExitCode(0)
	aliasSymlink.wantStdoutContains(`"status": "created"`)

	aliasHardlinkPath := aliasDir + "/alias.hard"
	aliasHardlink := runTDC(t, bin, "--profile", profileName, "fs", "hardlink", "--source-path", aliasPath, "--link-path", aliasHardlinkPath)
	aliasHardlink.wantExitCode(0)
	aliasHardlink.wantStdoutContains(`"status": "created"`)

	waitLiveFSResult(t, bin, []string{"--profile", profileName, "fs", "grep", "--path", aliasDir, "--pattern", "alias live e2e", "--limit", "5"}, "alias.txt", 2*time.Minute, "grep alias file content")
	waitLiveFSResult(t, bin, []string{"--profile", profileName, "fs", "find", "--path", aliasDir, "--file-name-pattern", "alias.txt", "--limit", "5"}, aliasPath, 2*time.Minute, "find alias file by name")

	aliasCopyPath := aliasDir + "/alias.copy.txt"
	aliasCopy := runTDC(t, bin, "--profile", profileName, "fs", "cp", "--from-remote", aliasPath, "--to-remote", aliasCopyPath)
	aliasCopy.wantExitCode(0)
	aliasCopy.wantStdoutContains(`"status": "copied"`)

	aliasMovedPath := aliasDir + "/alias.moved.txt"
	aliasMove := runTDC(t, bin, "--profile", profileName, "fs", "mv", "--from-remote", aliasCopyPath, "--to-remote", aliasMovedPath)
	aliasMove.wantExitCode(0)
	aliasMove.wantStdoutContains(`"status": "moved"`)

	aliasDelete := runTDC(t, bin, "--profile", profileName, "fs", "rm", "--path", aliasMovedPath)
	aliasDelete.wantExitCode(0)
	aliasDelete.wantStdoutContains(`"status": "deleted"`)

	largePath := rootPath + "/large.bin"
	largeContent := strings.Repeat("0123456789abcdef", 4096)
	largeFile := filepath.Join(t.TempDir(), "large.bin")
	if err := os.WriteFile(largeFile, []byte(largeContent), 0o644); err != nil {
		t.Fatalf("write large local file: %v", err)
	}
	largeUpload := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-local", largeFile, "--to-remote", largePath)
	largeUpload.wantExitCode(0)
	largeUpload.wantStdoutContains(`"status": "copied"`)
	largeUpload.wantStdoutContains(`"parts_uploaded": 1`)
	largeUpload.wantStdoutContains(`"upload_mode": "multipart_v2"`)
	readLarge := runTDC(t, bin, "--profile", profileName, "fs", "read-file", "--path", largePath)
	readLarge.wantExitCode(0)
	if readLarge.stdout != largeContent {
		readLarge.fail("large multipart upload should preserve file contents")
	}

	largeAppendText := strings.Repeat("append-"+suffix+"\n", 2048)
	largeAppendFile := filepath.Join(t.TempDir(), "large-append.txt")
	if err := os.WriteFile(largeAppendFile, []byte(largeAppendText), 0o644); err != nil {
		t.Fatalf("write large append file: %v", err)
	}
	largeAppend := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-local", largeAppendFile, "--to-remote", largePath, "--append")
	largeAppend.wantExitCode(0)
	largeAppend.wantStdoutContains(`"status": "appended"`)
	largeAppend.wantStdoutContains(`"parts_uploaded"`)
	largeAppend.wantStdoutContains(`"upload_mode": "append_patch"`)
	readLargeAppended := runTDC(t, bin, "--profile", profileName, "fs", "read-file", "--path", largePath)
	readLargeAppended.wantExitCode(0)
	if readLargeAppended.stdout != largeContent+largeAppendText {
		readLargeAppended.fail("efficient append should preserve existing and appended bytes")
	}

	resumeUploadPath := rootPath + "/resume-upload.bin"
	resumeUploadContent := strings.Repeat("resume-"+suffix+"-", 4096)
	resumeUploadFile := filepath.Join(t.TempDir(), "resume-upload.bin")
	if err := os.WriteFile(resumeUploadFile, []byte(resumeUploadContent), 0o644); err != nil {
		t.Fatalf("write resume upload file: %v", err)
	}
	fsClient := liveFSClient(t, profile, authz.FSFileWrite)
	resumeUploadID := initiateLiveUpload(t, fsClient, resumeUploadPath, resumeUploadFile)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = fsClient.AbortUpload(ctx, resumeUploadID)
	}()
	resumeUpload := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-local", resumeUploadFile, "--to-remote", resumeUploadPath, "--resume")
	resumeUpload.wantExitCode(0)
	resumeUpload.wantStdoutContains(`"status": "resumed"`)
	resumeUpload.wantStdoutContains(`"parts_uploaded": 1`)
	resumeUpload.wantStdoutContains(`"upload_mode": "resume_v1"`)
	readResumeUpload := runTDC(t, bin, "--profile", profileName, "fs", "read-file", "--path", resumeUploadPath)
	readResumeUpload.wantExitCode(0)
	if readResumeUpload.stdout != resumeUploadContent {
		readResumeUpload.fail("upload resume should preserve uploaded contents")
	}

	layerID := "tdc-e2e-layer-" + suffix
	checkpointID := layerID + "-cp"
	layerClosed := false
	defer func() {
		if layerClosed {
			return
		}
		rollback := runTDC(t, bin, "--profile", profileName, "fs", "rollback-layer", "--layer-id", layerID)
		if rollback.exitCode != 0 && rollback.exitCode != 5 {
			t.Logf("cleanup rollback failed for layer %s: exit=%d stdout=%s stderr=%s", layerID, rollback.exitCode, rollback.stdout, rollback.stderr)
		}
	}()
	createLayer := runTDC(
		t,
		bin,
		"--profile", profileName,
		"fs", "create-layer",
		"--layer-id", layerID,
		"--base-root-path", rootPath,
		"--layer-name", "live-e2e-"+suffix,
		"--durability-mode", "restore-safe",
		"--actor-id", "tdc-live-e2e",
		"--tag", "test=tdc-e2e",
		"--tag", "suffix="+suffix,
	)
	createLayer.wantExitCode(0)
	createLayer.wantStdoutContains(layerID)

	listLayers := runTDC(t, bin, "--profile", profileName, "fs", "list-layers", "--query", fmt.Sprintf("layers[?layer_id=='%s'].layer_id | [0]", layerID))
	listLayers.wantExitCode(0)
	listLayers.wantStdoutContains(layerID)

	describeLayer := runTDC(t, bin, "--profile", profileName, "fs", "describe-layer", "--layer-id", layerID)
	describeLayer.wantExitCode(0)
	describeLayer.wantStdoutContains(layerID)

	inlineLayerPath := rootPath + "/inline-layer.txt"
	inlineLayerContent := "inline layer content " + suffix
	createLayerEntry := runTDC(
		t,
		bin,
		"--profile", profileName,
		"fs", "create-layer-entry",
		"--layer-id", layerID,
		"--path", inlineLayerPath,
		"--operation", "upsert",
		"--resource-kind", "file",
		"--content", inlineLayerContent,
		"--content-type", "text/plain",
		"--content-text", inlineLayerContent,
		"--mode", "0644",
	)
	createLayerEntry.wantExitCode(0)
	createLayerEntry.wantStdoutContains(inlineLayerPath)

	layerUploadPath := rootPath + "/layer-upload.txt"
	layerUploadContent := "uploaded through layer " + suffix + "\n"
	layerUploadFile := filepath.Join(t.TempDir(), "layer-upload.txt")
	if err := os.WriteFile(layerUploadFile, []byte(layerUploadContent), 0o640); err != nil {
		t.Fatalf("write layer upload file: %v", err)
	}
	uploadLayerFile := runTDC(t, bin, "--profile", profileName, "fs", "upload-layer-file", "--layer-id", layerID, "--from-local", layerUploadFile, "--to-layer-path", layerUploadPath)
	uploadLayerFile.wantExitCode(0)
	uploadLayerFile.wantStdoutContains(layerUploadPath)

	layerCopyPath := rootPath + "/copy-layer.txt"
	layerCopyContent := "copy-file into layer " + suffix + "\n"
	layerCopyFile := filepath.Join(t.TempDir(), "copy-layer.txt")
	if err := os.WriteFile(layerCopyFile, []byte(layerCopyContent), 0o644); err != nil {
		t.Fatalf("write layer copy file: %v", err)
	}
	copyToLayer := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-local", layerCopyFile, "--to-remote", layerCopyPath, "--layer-id", layerID)
	copyToLayer.wantExitCode(0)
	copyToLayer.wantStdoutContains(`"status": "layered"`)

	readLayerFile := runTDC(t, bin, "--profile", profileName, "fs", "read-layer-file", "--layer-id", layerID, "--path", layerUploadPath)
	readLayerFile.wantExitCode(0)
	if readLayerFile.stdout != layerUploadContent {
		readLayerFile.fail("read-layer-file should return layer object bytes")
	}

	readLayerCopy := runTDC(t, bin, "--profile", profileName, "fs", "read-layer-file", "--layer-id", layerID, "--path", layerCopyPath)
	readLayerCopy.wantExitCode(0)
	if readLayerCopy.stdout != layerCopyContent {
		readLayerCopy.fail("copy-file --layer-id should write readable layer bytes")
	}

	describeLayerEntry := runTDC(t, bin, "--profile", profileName, "fs", "describe-layer-entry", "--layer-id", layerID, "--path", layerUploadPath)
	describeLayerEntry.wantExitCode(0)
	describeLayerEntry.wantStdoutContains(layerUploadPath)

	diffLayer := runTDC(t, bin, "--profile", profileName, "fs", "diff-layer", "--layer-id", layerID, "--output", "human")
	diffLayer.wantExitCode(0)
	diffLayer.wantStdoutContains(layerUploadPath)
	diffLayer.wantStdoutContains(layerCopyPath)

	replayLayer := runTDC(t, bin, "--profile", profileName, "fs", "replay-layer", "--layer-id", layerID)
	replayLayer.wantExitCode(0)
	replayLayer.wantStdoutContains(layerUploadPath)

	createCheckpoint := runTDC(t, bin, "--profile", profileName, "fs", "create-layer-checkpoint", "--layer-id", layerID, "--checkpoint-id", checkpointID, "--label", "live-e2e")
	createCheckpoint.wantExitCode(0)
	createCheckpoint.wantStdoutContains(checkpointID)

	describeCheckpoint := runTDC(t, bin, "--profile", profileName, "fs", "describe-layer-checkpoint", "--checkpoint-id", checkpointID)
	describeCheckpoint.wantExitCode(0)
	describeCheckpoint.wantStdoutContains(checkpointID)

	listLayerEvents := runTDC(t, bin, "--profile", profileName, "fs", "list-layer-events", "--layer-id", layerID)
	listLayerEvents.wantExitCode(0)
	listLayerEvents.wantStdoutContains(layerUploadPath)

	waitLiveFSResult(t, bin, []string{"--profile", profileName, "fs", "find-files", "--path", rootPath, "--file-name-pattern", "copy-layer.txt", "--layer-id", layerID, "--limit", "5"}, layerCopyPath, 2*time.Minute, "find file inside layer")

	commitLayer := runTDC(t, bin, "--profile", profileName, "fs", "commit-layer", "--layer-id", layerID)
	commitLayer.wantExitCode(0)
	commitLayer.wantStdoutContains(`"status"`)
	layerClosed = true
	waitLiveRemoteRead(t, bin, profileName, layerUploadPath, layerUploadContent, 30*time.Second)
	waitLiveRemoteRead(t, bin, profileName, layerCopyPath, layerCopyContent, 30*time.Second)

	vaultSecretName := "tdc-e2e-vault-" + suffix
	vaultDeleted := false
	defer func() {
		if vaultDeleted {
			return
		}
		cleanup := runTDC(t, bin, "--profile", profileName, "vault", "delete-secret", "--secret-name", vaultSecretName)
		if cleanup.exitCode != 0 && cleanup.exitCode != 5 {
			t.Logf("cleanup vault secret failed for %s: exit=%d stdout=%s stderr=%s", vaultSecretName, cleanup.exitCode, cleanup.stdout, cleanup.stderr)
		}
	}()
	createVaultSecret := runTDC(
		t,
		bin,
		"--profile", profileName,
		"vault", "create-secret",
		"--secret-name", vaultSecretName,
		"--field", "DB_URL=mysql://"+suffix,
		"--field", "PASSWORD=secret-"+suffix,
	)
	createVaultSecret.wantExitCode(0)
	createVaultSecret.wantStdoutContains(vaultSecretName)

	listVaultSecrets := runTDC(t, bin, "--profile", profileName, "vault", "list-secrets")
	listVaultSecrets.wantExitCode(0)
	listVaultSecrets.wantStdoutContains(vaultSecretName)

	readVaultSecret := runTDC(t, bin, "--profile", profileName, "vault", "read-secret", "--secret-name", vaultSecretName, "--field", "PASSWORD", "--format", "raw")
	readVaultSecret.wantExitCode(0)
	if readVaultSecret.stdout != "secret-"+suffix {
		readVaultSecret.fail("vault read-secret --format raw should return exact field bytes")
	}

	readVaultEnv := runTDC(t, bin, "--profile", profileName, "vault", "read-secret", "--secret-name", vaultSecretName, "--format", "env")
	readVaultEnv.wantExitCode(0)
	readVaultEnv.wantStdoutContains("DB_URL=mysql://" + suffix)
	readVaultEnv.wantStdoutContains("PASSWORD=secret-" + suffix)

	replaceVaultDir := filepath.Join(t.TempDir(), "vault-replace")
	if err := os.MkdirAll(replaceVaultDir, 0o755); err != nil {
		t.Fatalf("create vault replace dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(replaceVaultDir, "DB_URL"), []byte("mysql://replaced-"+suffix), 0o600); err != nil {
		t.Fatalf("write replacement DB_URL: %v", err)
	}
	if err := os.WriteFile(filepath.Join(replaceVaultDir, "PASSWORD"), []byte("replaced-"+suffix), 0o600); err != nil {
		t.Fatalf("write replacement PASSWORD: %v", err)
	}
	replaceVaultSecret := runTDC(t, bin, "--profile", profileName, "vault", "replace-secret", "--secret-path", "/n/vault/"+vaultSecretName, "--from-directory", replaceVaultDir)
	replaceVaultSecret.wantExitCode(0)
	replaceVaultSecret.wantStdoutContains(vaultSecretName)

	readReplacedVaultSecret := runTDC(t, bin, "--profile", profileName, "vault", "read-secret", "--secret-name", vaultSecretName, "--field", "PASSWORD", "--format", "raw")
	readReplacedVaultSecret.wantExitCode(0)
	if readReplacedVaultSecret.stdout != "replaced-"+suffix {
		readReplacedVaultSecret.fail("vault replace-secret should replace stored field bytes")
	}

	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		vaultMountPath := filepath.Join(t.TempDir(), "vault-mount")
		if err := os.MkdirAll(vaultMountPath, 0o755); err != nil {
			t.Fatalf("create vault mount path: %v", err)
		}
		vaultUnmounted := false
		defer func() {
			if vaultUnmounted {
				return
			}
			cleanupUnmount := runTDC(t, bin, "--profile", profileName, "vault", "unmount-vault", "--mount-path", vaultMountPath, "--ignore-absent", "--force")
			if cleanupUnmount.exitCode != 0 {
				t.Logf("cleanup vault unmount failed for %s: exit=%d stdout=%s stderr=%s", vaultMountPath, cleanupUnmount.exitCode, cleanupUnmount.stdout, cleanupUnmount.stderr)
			}
		}()
		mountVault := runTDC(t, bin, "--profile", profileName, "vault", "mount-vault", "--mount-path", vaultMountPath, "--ready-timeout", "30s")
		mountVault.wantExitCode(0)
		mountVault.wantStdoutContains(`"status": "mounted"`)
		waitLiveLocalFile(t, filepath.Join(vaultMountPath, vaultSecretName, "PASSWORD"), "replaced-"+suffix, 30*time.Second)
		unmountVault := runTDC(t, bin, "--profile", profileName, "vault", "unmount-vault", "--mount-path", vaultMountPath)
		unmountVault.wantExitCode(0)
		unmountVault.wantStdoutContains(`"status": "unmounted"`)
		vaultUnmounted = true
	}

	createVaultGrant := runTDC(
		t,
		bin,
		"--profile", profileName,
		"vault", "create-grant",
		"--agent-id", "tdc-live-e2e",
		"--scope", vaultSecretName+"/DB_URL",
		"--permission", "read",
		"--ttl", "10m",
		"--label-hint", "tdc-live-e2e-"+suffix,
	)
	createVaultGrant.wantExitCode(0)
	vaultGrant := decodeLiveVaultToken(t, createVaultGrant)
	if vaultGrant.Token == "" || vaultGrant.GrantID == "" {
		t.Fatalf("unexpected vault grant response: %#v\n%s", vaultGrant, createVaultGrant.stdout)
	}
	grantDeleted := false
	defer func() {
		if grantDeleted {
			return
		}
		cleanup := runTDC(t, bin, "--profile", profileName, "vault", "delete-grant", "--grant-id", vaultGrant.GrantID, "--reason", "cleanup")
		if cleanup.exitCode != 0 && cleanup.exitCode != 5 {
			t.Logf("cleanup vault grant failed for %s: exit=%d stdout=%s stderr=%s", vaultGrant.GrantID, cleanup.exitCode, cleanup.stdout, cleanup.stderr)
		}
	}()
	readVaultWithGrant := runTDC(t, bin, "--profile", profileName, "vault", "read-secret", "--secret-name", vaultSecretName, "--field", "DB_URL", "--format", "raw", "--vault-token", vaultGrant.Token)
	readVaultWithGrant.wantExitCode(0)
	if readVaultWithGrant.stdout != "mysql://replaced-"+suffix {
		readVaultWithGrant.fail("delegated vault grant should read scoped field")
	}
	deleteVaultGrant := runTDC(t, bin, "--profile", profileName, "vault", "delete-grant", "--grant-id", vaultGrant.GrantID, "--reason", "live-e2e-complete")
	deleteVaultGrant.wantExitCode(0)
	grantDeleted = true

	createVaultToken := runTDC(
		t,
		bin,
		"--profile", profileName,
		"vault", "create-token",
		"--agent-id", "tdc-live-e2e",
		"--task-id", suffix,
		"--scope", vaultSecretName,
		"--ttl", "10m",
	)
	createVaultToken.wantExitCode(0)
	vaultToken := decodeLiveVaultToken(t, createVaultToken)
	if vaultToken.Token == "" || vaultToken.TokenID == "" {
		t.Fatalf("unexpected vault token response: %#v\n%s", vaultToken, createVaultToken.stdout)
	}
	tokenDeleted := false
	defer func() {
		if tokenDeleted {
			return
		}
		cleanup := runTDC(t, bin, "--profile", profileName, "vault", "delete-token", "--token-id", vaultToken.TokenID)
		if cleanup.exitCode != 0 && cleanup.exitCode != 5 {
			t.Logf("cleanup vault token failed for %s: exit=%d stdout=%s stderr=%s", vaultToken.TokenID, cleanup.exitCode, cleanup.stdout, cleanup.stderr)
		}
	}()
	readVaultWithToken := runTDC(t, bin, "--profile", profileName, "vault", "read-secret", "--secret-name", vaultSecretName, "--field", "PASSWORD", "--format", "raw", "--vault-token", vaultToken.Token)
	readVaultWithToken.wantExitCode(0)
	if readVaultWithToken.stdout != "replaced-"+suffix {
		readVaultWithToken.fail("legacy vault token should read scoped field")
	}
	deleteVaultToken := runTDC(t, bin, "--profile", profileName, "vault", "delete-token", "--token-id", vaultToken.TokenID)
	deleteVaultToken.wantExitCode(0)
	tokenDeleted = true

	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath("env"); err == nil {
			runWithVault := runTDC(t, bin, "--profile", profileName, "vault", "run-with-secret", "--secret-path", "/n/vault/"+vaultSecretName, "--", "env")
			runWithVault.wantExitCode(0)
			runWithVault.wantStdoutContains("DB_URL=mysql://replaced-" + suffix)
			runWithVault.wantStdoutContains("PASSWORD=replaced-" + suffix)
			runWithVault.wantStdoutNotContains("TDC_PRIVATE_KEY=")
		}
	}

	listVaultAuditEvents := runTDC(t, bin, "--profile", profileName, "vault", "list-audit-events", "--secret-name", vaultSecretName, "--limit", "20")
	listVaultAuditEvents.wantExitCode(0)
	listVaultAuditEvents.wantStdoutContains(`"events"`)

	deleteVaultSecret := runTDC(t, bin, "--profile", profileName, "vault", "delete-secret", "--secret-name", vaultSecretName)
	deleteVaultSecret.wantExitCode(0)
	deleteVaultSecret.wantStdoutContains(`"status": "deleted"`)
	vaultDeleted = true

	journalID := "jrn-tdc-e2e-" + suffix
	appendID := "app-tdc-e2e-" + suffix
	createJournal := runTDC(
		t,
		bin,
		"--profile", profileName,
		"journal", "create-journal",
		"--journal-id", journalID,
		"--journal-kind", "agent",
		"--title", "tdc live e2e "+suffix,
		"--actor", "agent:tdc-live-e2e",
		"--label", "test=tdc-e2e",
		"--label", "suffix="+suffix,
	)
	createJournal.wantExitCode(0)
	createJournal.wantStdoutContains(journalID)

	appendJournal := runTDC(
		t,
		bin,
		"--profile", profileName,
		"journal", "append-journal-entries",
		"--journal-id", journalID,
		"--idempotency-key", appendID,
		"--entry-json", `{"type":"task.started","summary":{"message":"tdc live e2e `+suffix+`"}}`,
		"--subject", "path:"+rootPath,
	)
	appendJournal.wantExitCode(0)
	appendJournal.wantStdoutContains(`"count": 1`)
	appendJournal.wantStdoutContains(appendID)

	readJournal := runTDC(t, bin, "--profile", profileName, "journal", "read-journal-entries", "--journal-id", journalID, "--limit", "10")
	readJournal.wantExitCode(0)
	readJournal.wantStdoutContains(journalID)
	readJournal.wantStdoutContains("task.started")

	waitLiveFSResult(
		t,
		bin,
		[]string{
			"--profile", profileName,
			"journal", "search-journal-entries",
			"--entry-type", "task.started",
			"--label", "test=tdc-e2e",
			"--limit", "10",
		},
		journalID,
		2*time.Minute,
		"index journal entry",
	)

	verifyJournal := runTDC(t, bin, "--profile", profileName, "journal", "verify-journal", "--journal-id", journalID, "--output", "human")
	verifyJournal.wantExitCode(0)
	verifyJournal.wantStdoutContains("ok journal=" + journalID)

	gitRootPath := rootPath + "/git-workspace"
	gitCommit := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	gitObject := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	createGitWorkspace := runTDC(
		t,
		bin,
		"--profile", profileName,
		"git", "create-git-workspace",
		"--root-path", gitRootPath,
		"--repo-url", "https://example.test/tdc-e2e.git",
		"--remote-name", "origin",
		"--branch-name", "main",
		"--base-commit", gitCommit,
		"--head-commit", gitCommit,
		"--mode", "fast",
		"--workspace-kind", "main",
	)
	createGitWorkspace.wantExitCode(0)
	gitWorkspace := decodeLiveGitWorkspace(t, createGitWorkspace)
	if gitWorkspace.WorkspaceID == "" {
		t.Fatalf("unexpected git workspace response: %#v\n%s", gitWorkspace, createGitWorkspace.stdout)
	}
	gitWorkspaceDeleted := false
	defer func() {
		if gitWorkspaceDeleted {
			return
		}
		cleanup := runTDC(t, bin, "--profile", profileName, "git", "delete-git-workspace", "--workspace-id", gitWorkspace.WorkspaceID)
		if cleanup.exitCode != 0 && cleanup.exitCode != 5 {
			t.Logf("cleanup git workspace failed for %s: exit=%d stdout=%s stderr=%s", gitWorkspace.WorkspaceID, cleanup.exitCode, cleanup.stdout, cleanup.stderr)
		}
	}()

	listGitWorkspaces := runTDC(t, bin, "--profile", profileName, "git", "list-git-workspaces", "--query", fmt.Sprintf("workspaces[?workspace_id=='%s'].workspace_id | [0]", gitWorkspace.WorkspaceID))
	listGitWorkspaces.wantExitCode(0)
	listGitWorkspaces.wantStdoutContains(gitWorkspace.WorkspaceID)

	describeGitByID := runTDC(t, bin, "--profile", profileName, "git", "describe-git-workspace", "--workspace-id", gitWorkspace.WorkspaceID)
	describeGitByID.wantExitCode(0)
	describeGitByID.wantStdoutContains(gitRootPath)

	describeGitByRoot := runTDC(t, bin, "--profile", profileName, "git", "describe-git-workspace", "--root-path", gitRootPath)
	describeGitByRoot.wantExitCode(0)
	describeGitByRoot.wantStdoutContains(gitWorkspace.WorkspaceID)

	nodeJSON := fmt.Sprintf(`{"path":"README.md","parent_path":"","name":"README.md","kind":"file","mode":"100644","object_sha":"%s","size_bytes":5}`, gitObject)
	replaceGitTree := runTDC(t, bin, "--profile", profileName, "git", "replace-git-tree", "--workspace-id", gitWorkspace.WorkspaceID, "--commit-sha", gitCommit, "--node-json", nodeJSON)
	replaceGitTree.wantExitCode(0)
	replaceGitTree.wantStdoutContains(`"status": "replaced"`)

	listGitTree := runTDC(t, bin, "--profile", profileName, "git", "list-git-tree", "--workspace-id", gitWorkspace.WorkspaceID, "--commit-sha", gitCommit)
	listGitTree.wantExitCode(0)
	listGitTree.wantStdoutContains("README.md")

	upsertGitState := runTDC(t, bin, "--profile", profileName, "git", "upsert-git-state", "--workspace-id", gitWorkspace.WorkspaceID, "--checkpoint-commit", gitCommit, "--storage-type", "inline", "--content", "state-"+suffix)
	upsertGitState.wantExitCode(0)
	upsertGitState.wantStdoutContains(gitWorkspace.WorkspaceID)

	describeGitState := runTDC(t, bin, "--profile", profileName, "git", "describe-git-state", "--workspace-id", gitWorkspace.WorkspaceID)
	describeGitState.wantExitCode(0)
	describeGitState.wantStdoutContains(gitCommit)

	putGitObjectPack := runTDC(t, bin, "--profile", profileName, "git", "put-git-object-pack", "--workspace-id", gitWorkspace.WorkspaceID, "--content", "pack-"+suffix)
	putGitObjectPack.wantExitCode(0)
	gitObjectPack := decodeLiveGitObjectPack(t, putGitObjectPack)
	if gitObjectPack.PackID == "" {
		t.Fatalf("unexpected git object pack response: %#v\n%s", gitObjectPack, putGitObjectPack.stdout)
	}

	listGitObjectPacks := runTDC(t, bin, "--profile", profileName, "git", "list-git-object-packs", "--workspace-id", gitWorkspace.WorkspaceID)
	listGitObjectPacks.wantExitCode(0)
	listGitObjectPacks.wantStdoutContains(gitObjectPack.PackID)

	describeGitObjectPack := runTDC(t, bin, "--profile", profileName, "git", "describe-git-object-pack", "--workspace-id", gitWorkspace.WorkspaceID, "--pack-id", gitObjectPack.PackID)
	describeGitObjectPack.wantExitCode(0)
	describeGitObjectPack.wantStdoutContains(gitObjectPack.PackID)

	putGitOverlay := runTDC(t, bin, "--profile", profileName, "git", "put-git-overlay-entry", "--workspace-id", gitWorkspace.WorkspaceID, "--path", "README.md", "--operation", "upsert", "--resource-kind", "file", "--mode", "100644", "--content", "hello "+suffix)
	putGitOverlay.wantExitCode(0)
	putGitOverlay.wantStdoutContains("README.md")

	describeGitOverlay := runTDC(t, bin, "--profile", profileName, "git", "describe-git-overlay-entry", "--workspace-id", gitWorkspace.WorkspaceID, "--path", "README.md")
	describeGitOverlay.wantExitCode(0)
	describeGitOverlay.wantStdoutContains("README.md")

	listGitOverlay := runTDC(t, bin, "--profile", profileName, "git", "list-git-overlay-entries", "--workspace-id", gitWorkspace.WorkspaceID)
	listGitOverlay.wantExitCode(0)
	listGitOverlay.wantStdoutContains("README.md")

	deleteGitWorkspace := runTDC(t, bin, "--profile", profileName, "git", "delete-git-workspace", "--workspace-id", gitWorkspace.WorkspaceID)
	deleteGitWorkspace.wantExitCode(0)
	deleteGitWorkspace.wantStdoutContains(`"status": "deleted"`)
	gitWorkspaceDeleted = true

	waitLiveFSResult(t, bin, []string{"--profile", profileName, "fs", "search-file-content", "--path", rootPath, "--pattern", "tdc fs live e2e", "--limit", "5"}, "README.md", 2*time.Minute, "find uploaded file content")
	waitLiveFSResult(t, bin, []string{"--profile", profileName, "fs", "find-files", "--path", rootPath, "--file-name-pattern", "*.md", "--limit", "5"}, "README.md", 2*time.Minute, "find uploaded file by name")

	remoteCopy := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-remote", sourcePath, "--to-remote", copyPath)
	remoteCopy.wantExitCode(0)
	remoteCopy.wantStdoutContains(`"status": "copied"`)

	move := runTDC(t, bin, "--profile", profileName, "fs", "move-file", "--from-remote", copyPath, "--to-remote", movedPath)
	move.wantExitCode(0)
	move.wantStdoutContains(`"status": "moved"`)

	downloadPath := filepath.Join(t.TempDir(), "nested", "downloaded.md")
	download := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-remote", movedPath, "--to-local", downloadPath, "--create-parents")
	download.wantExitCode(0)
	download.wantStdoutContains(`"status": "copied"`)
	downloaded, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(downloaded) != fullContent {
		t.Fatalf("downloaded file mismatch: got %q want %q", downloaded, fullContent)
	}

	resumePath := filepath.Join(t.TempDir(), "resume.md")
	if err := os.WriteFile(resumePath, []byte(fullContent[:5]), 0o644); err != nil {
		t.Fatalf("write partial resume file: %v", err)
	}
	resume := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-remote", movedPath, "--to-local", resumePath, "--resume")
	resume.wantExitCode(0)
	resume.wantStdoutContains(`"status": "resumed"`)
	resumed, err := os.ReadFile(resumePath)
	if err != nil {
		t.Fatalf("read resumed file: %v", err)
	}
	if string(resumed) != fullContent {
		t.Fatalf("resumed file mismatch: got %q want %q", resumed, fullContent)
	}

	localTree := filepath.Join(t.TempDir(), "tree")
	if err := os.MkdirAll(filepath.Join(localTree, "nested"), 0o755); err != nil {
		t.Fatalf("create local tree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localTree, "alpha.txt"), []byte("alpha "+suffix), 0o644); err != nil {
		t.Fatalf("write local tree file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localTree, "nested", "beta.txt"), []byte("beta "+suffix), 0o644); err != nil {
		t.Fatalf("write nested local tree file: %v", err)
	}
	treeRoot := rootPath + "/tree"
	recursiveUpload := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-local", localTree, "--to-remote", treeRoot, "--recursive")
	recursiveUpload.wantExitCode(0)
	recursiveUpload.wantStdoutContains(`"files_transferred": 2`)
	readTreeFile := runTDC(t, bin, "--profile", profileName, "fs", "read-file", "--path", treeRoot+"/nested/beta.txt")
	readTreeFile.wantExitCode(0)
	if readTreeFile.stdout != "beta "+suffix {
		readTreeFile.fail("recursive local-to-remote copy should preserve nested file contents")
	}

	treeCopyRoot := rootPath + "/tree-copy"
	recursiveRemoteCopy := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-remote", treeRoot, "--to-remote", treeCopyRoot, "--recursive")
	recursiveRemoteCopy.wantExitCode(0)
	recursiveRemoteCopy.wantStdoutContains(`"files_transferred": 2`)
	readCopiedTreeFile := runTDC(t, bin, "--profile", profileName, "fs", "read-file", "--path", treeCopyRoot+"/nested/beta.txt")
	readCopiedTreeFile.wantExitCode(0)
	if readCopiedTreeFile.stdout != "beta "+suffix {
		readCopiedTreeFile.fail("recursive remote-to-remote copy should preserve nested file contents")
	}

	downloadTree := filepath.Join(t.TempDir(), "download-tree")
	recursiveDownload := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-remote", treeRoot, "--to-local", downloadTree, "--recursive")
	recursiveDownload.wantExitCode(0)
	recursiveDownload.wantStdoutContains(`"files_transferred": 2`)
	downloadedTreeFile, err := os.ReadFile(filepath.Join(downloadTree, "nested", "beta.txt"))
	if err != nil {
		t.Fatalf("read recursive download file: %v", err)
	}
	if string(downloadedTreeFile) != "beta "+suffix {
		t.Fatalf("recursive download mismatch: got %q", downloadedTreeFile)
	}

	packLocalRoot := t.TempDir()
	packOverlayFile := filepath.Join(packLocalRoot, "overlay", "repo", "cache", "item.txt")
	if err := os.MkdirAll(filepath.Dir(packOverlayFile), 0o755); err != nil {
		t.Fatalf("create pack overlay dir: %v", err)
	}
	packContent := "pack portable overlay " + suffix + "\n"
	if err := os.WriteFile(packOverlayFile, []byte(packContent), 0o644); err != nil {
		t.Fatalf("write pack overlay file: %v", err)
	}
	packArchivePath := rootPath + "/packs/portable.tar.gz"
	pack := runTDC(t, bin, "--profile", profileName, "fs", "pack-file-system", "--local-root", packLocalRoot, "--remote-root", rootPath, "--mount-profile", "portable", "--archive-path", packArchivePath)
	pack.wantExitCode(0)
	pack.wantStdoutContains(`"status": "packed"`)
	pack.wantStdoutContains(`"archive_path": "` + packArchivePath + `"`)
	unpackLocalRoot := t.TempDir()
	unpack := runTDC(t, bin, "--profile", profileName, "fs", "unpack-file-system", "--local-root", unpackLocalRoot, "--remote-root", rootPath, "--mount-profile", "portable", "--archive-path", packArchivePath)
	unpack.wantExitCode(0)
	unpack.wantStdoutContains(`"status": "unpacked"`)
	unpackedPackFile, err := os.ReadFile(filepath.Join(unpackLocalRoot, "overlay", "repo", "cache", "item.txt"))
	if err != nil {
		t.Fatalf("read unpacked pack file: %v", err)
	}
	if string(unpackedPackFile) != packContent {
		t.Fatalf("unpacked pack content mismatch: got %q want %q", unpackedPackFile, packContent)
	}

	deleteMoved := runTDC(t, bin, "--profile", profileName, "fs", "delete-file", "--path", movedPath)
	deleteMoved.wantExitCode(0)
	deleteMoved.wantStdoutContains(`"status": "deleted"`)

	deleteRoot := runTDC(t, bin, "--profile", profileName, "fs", "delete-file", "--path", rootPath, "--recursive")
	deleteRoot.wantExitCode(0)
	deleteRoot.wantStdoutContains(`"status": "deleted"`)
	deleted = true
}

func TestLiveFSMountRuntime(t *testing.T) {
	requireLive(t)
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("tdc fs FUSE mount live e2e currently runs on macOS or Linux")
	}
	profile := liveProfile(t)
	if profile.FSAPIKey == "" {
		t.Fatalf("live profile %q has no fs_api_key; run tdc fs create-file-system --profile %s before make live-e2e", profile.Name, profile.Name)
	}

	bin := tdcBinary(t)
	profileName := liveProfileName(t)
	suffix := time.Now().UTC().Format("20060102150405")
	remoteRoot := "/tdc-e2e-mount-" + suffix
	mountPath := filepath.Join(t.TempDir(), "mount")
	if err := os.MkdirAll(mountPath, 0o755); err != nil {
		t.Fatalf("create mount path: %v", err)
	}
	unmounted := false
	remoteDeleted := false
	defer func() {
		if !unmounted {
			cleanupUnmount := runTDC(t, bin, "--profile", profileName, "fs", "unmount-file-system", "--mount-path", mountPath, "--ignore-absent", "--force")
			if cleanupUnmount.exitCode != 0 {
				t.Logf("cleanup unmount failed for %s: exit=%d stdout=%s stderr=%s", mountPath, cleanupUnmount.exitCode, cleanupUnmount.stdout, cleanupUnmount.stderr)
			}
		}
		if !remoteDeleted {
			cleanupRemote := runTDC(t, bin, "--profile", profileName, "fs", "delete-file", "--path", remoteRoot, "--recursive")
			if cleanupRemote.exitCode != 0 && cleanupRemote.exitCode != 5 {
				t.Logf("cleanup remote failed for %s: exit=%d stdout=%s stderr=%s", remoteRoot, cleanupRemote.exitCode, cleanupRemote.stdout, cleanupRemote.stderr)
			}
		}
	}()

	createDir := runTDC(t, bin, "--profile", profileName, "fs", "create-directory", "--path", remoteRoot, "--mode", "0755")
	createDir.wantExitCode(0)
	localSeed := filepath.Join(t.TempDir(), "README.md")
	seedContent := "hello mounted tdc fs " + suffix + "\n"
	if err := os.WriteFile(localSeed, []byte(seedContent), 0o644); err != nil {
		t.Fatalf("write local seed: %v", err)
	}
	upload := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-local", localSeed, "--to-remote", remoteRoot+"/README.md")
	upload.wantExitCode(0)

	mount := runTDC(t, bin, "--profile", profileName, "fs", "mount", "--mount-path", mountPath, "--remote-path", remoteRoot, "--ready-timeout", "30s")
	mount.wantExitCode(0)
	mount.wantStdoutContains(`"status": "mounted"`)
	mount.wantStdoutContains(`"driver": "fuse"`)
	mount.wantStdoutContains(`"control_socket"`)

	waitLiveLocalFile(t, filepath.Join(mountPath, "README.md"), seedContent, 30*time.Second)
	localWrite := "written through mounted tdc fs " + suffix + "\n"
	if err := os.WriteFile(filepath.Join(mountPath, "local-write.txt"), []byte(localWrite), 0o644); err != nil {
		t.Fatalf("write through mount failed: %v", err)
	}
	drain := runTDC(t, bin, "--profile", profileName, "fs", "drain", "--mount-path", mountPath, "--timeout", "30s")
	drain.wantExitCode(0)
	drain.wantStdoutContains(`"status": "drained"`)
	drain.wantStdoutContains(`"ok": true`)
	waitLiveRemoteRead(t, bin, profileName, remoteRoot+"/local-write.txt", localWrite, 30*time.Second)

	unmount := runTDC(t, bin, "--profile", profileName, "fs", "umount", "--mount-path", mountPath)
	unmount.wantExitCode(0)
	unmount.wantStdoutContains(`"status": "unmounted"`)
	unmounted = true

	deleteRoot := runTDC(t, bin, "--profile", profileName, "fs", "delete-file", "--path", remoteRoot, "--recursive")
	deleteRoot.wantExitCode(0)
	remoteDeleted = true
}

func TestLiveFSWebDAVMountRuntime(t *testing.T) {
	requireLive(t)
	if runtime.GOOS != "darwin" {
		t.Skip("tdc fs WebDAV mount live e2e currently runs on macOS")
	}
	if _, err := exec.LookPath("mount_webdav"); err != nil {
		t.Skip("mount_webdav is not available")
	}
	if _, err := exec.LookPath("umount"); err != nil {
		t.Skip("umount is not available")
	}
	profile := liveProfile(t)
	if profile.FSAPIKey == "" {
		t.Fatalf("live profile %q has no fs_api_key; run tdc fs create-file-system --profile %s before make live-e2e", profile.Name, profile.Name)
	}

	bin := tdcBinary(t)
	profileName := liveProfileName(t)
	suffix := time.Now().UTC().Format("20060102150405")
	remoteRoot := "/tdc-e2e-webdav-mount-" + suffix
	mountPath := filepath.Join(t.TempDir(), "mount")
	if err := os.MkdirAll(mountPath, 0o755); err != nil {
		t.Fatalf("create mount path: %v", err)
	}
	unmounted := false
	remoteDeleted := false
	defer func() {
		if !unmounted {
			cleanupUnmount := runTDC(t, bin, "--profile", profileName, "fs", "unmount-file-system", "--mount-path", mountPath, "--ignore-absent", "--force")
			if cleanupUnmount.exitCode != 0 {
				t.Logf("cleanup unmount failed for %s: exit=%d stdout=%s stderr=%s", mountPath, cleanupUnmount.exitCode, cleanupUnmount.stdout, cleanupUnmount.stderr)
			}
		}
		if !remoteDeleted {
			cleanupRemote := runTDC(t, bin, "--profile", profileName, "fs", "delete-file", "--path", remoteRoot, "--recursive")
			if cleanupRemote.exitCode != 0 && cleanupRemote.exitCode != 5 {
				t.Logf("cleanup remote failed for %s: exit=%d stdout=%s stderr=%s", remoteRoot, cleanupRemote.exitCode, cleanupRemote.stdout, cleanupRemote.stderr)
			}
		}
	}()

	createDir := runTDC(t, bin, "--profile", profileName, "fs", "create-directory", "--path", remoteRoot, "--mode", "0755")
	createDir.wantExitCode(0)
	localSeed := filepath.Join(t.TempDir(), "README.md")
	seedContent := "hello webdav mounted tdc fs " + suffix + "\n"
	if err := os.WriteFile(localSeed, []byte(seedContent), 0o644); err != nil {
		t.Fatalf("write local seed: %v", err)
	}
	upload := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-local", localSeed, "--to-remote", remoteRoot+"/README.md")
	upload.wantExitCode(0)

	mount := runTDC(t, bin, "--profile", profileName, "fs", "mount-file-system", "--mount-path", mountPath, "--remote-path", remoteRoot, "--driver", "webdav", "--ready-timeout", "30s")
	mount.wantExitCode(0)
	mount.wantStdoutContains(`"status": "mounted"`)
	mount.wantStdoutContains(`"driver": "webdav"`)

	waitLiveLocalFile(t, filepath.Join(mountPath, "README.md"), seedContent, 30*time.Second)

	unmount := runTDC(t, bin, "--profile", profileName, "fs", "unmount-file-system", "--mount-path", mountPath)
	unmount.wantExitCode(0)
	unmount.wantStdoutContains(`"status": "unmounted"`)
	unmounted = true

	deleteRoot := runTDC(t, bin, "--profile", profileName, "fs", "delete-file", "--path", remoteRoot, "--recursive")
	deleteRoot.wantExitCode(0)
	remoteDeleted = true
}

func TestLiveDBClusterLifecycle(t *testing.T) {
	requireLive(t)

	bin := tdcBinary(t)
	profileName := liveProfileName(t)
	projectID := liveProjectID(t, bin, profileName)

	suffix := time.Now().UTC().Format("20060102150405")
	clusterName := "tdc-e2e-" + suffix
	updatedName := clusterName + "-u"
	var clusterID string
	currentName := clusterName
	deleted := false
	defer func() {
		if clusterID == "" || deleted {
			return
		}
		cleanup := runTDC(t, bin, "--profile", profileName, "db", "delete-db-cluster", "--db-cluster-id", clusterID, "--confirm-db-cluster-name", currentName)
		if cleanup.exitCode != 0 && cleanup.exitCode != 5 {
			t.Logf("cleanup delete failed for cluster %s: exit=%d stdout=%s stderr=%s", clusterID, cleanup.exitCode, cleanup.stdout, cleanup.stderr)
		}
	}()

	create := runTDC(
		t,
		bin,
		"--profile", profileName,
		"db", "create-db-cluster",
		"--db-cluster-name", clusterName,
		"--db-cluster-type", "starter",
		"--project-id", projectID,
	)
	create.wantExitCode(0)
	created := decodeLiveCluster(t, create)
	if created.ID == "" || created.DisplayName != clusterName {
		t.Fatalf("unexpected created cluster: %#v\n%s", created, create.stdout)
	}
	clusterID = created.ID
	defer cleanupLiveSQLCredentials(t, clusterID)

	described := waitLiveCluster(t, bin, profileName, clusterID, func(cluster liveCluster) bool {
		return cluster.ID == clusterID && cluster.DisplayName == clusterName && cluster.State == "ACTIVE"
	}, 12*time.Minute, "become ACTIVE after create")
	if described.ClusterPlan != "" && described.ClusterPlan != "STARTER" {
		t.Fatalf("expected STARTER cluster, got %#v", described)
	}

	prepare := runTDC(t, bin, "--profile", profileName, "db", "prepare-db-query-access", "--db-cluster-id", clusterID)
	prepare.wantExitCode(0)
	prepare.wantStdoutContains(`"read_only"`)
	prepare.wantStdoutContains(`"read_write"`)
	prepare.wantStdoutContains(`"admin"`)

	prepareAgain := runTDC(t, bin, "--profile", profileName, "db", "prepare-db-query-access", "--db-cluster-id", clusterID)
	prepareAgain.wantExitCode(0)
	prepareAgain.wantStdoutContains(`"exists"`)

	connectionString := runTDC(t, bin, "--profile", profileName, "db", "create-db-connection-string", "--db-cluster-id", clusterID, "--read-write", "--database", "test")
	connectionString.wantExitCode(0)
	connectionString.wantStdoutContains(`"format": "mysql-uri"`)
	connectionString.wantStdoutContains(`"access_mode": "read_write"`)
	connectionString.wantStdoutContains(`"connection_string"`)

	connectionEnv := runTDC(t, bin, "--profile", profileName, "db", "create-db-connection-string", "--db-cluster-id", clusterID, "--read-only", "--format", "env")
	connectionEnv.wantExitCode(0)
	connectionEnv.wantStdoutContains("TIDB_HOST=")
	connectionEnv.wantStdoutContains("TIDB_ACCESS_MODE=read_only")
	connectionEnv.wantStdoutNotContains(`"connection_string"`)

	waitLiveSQL(t, bin, profileName, clusterID, nil, "default read-write SQL execution")
	waitLiveSQL(t, bin, profileName, clusterID, []string{"--read-only"}, "read-only SQL execution")
	waitLiveSQL(t, bin, profileName, clusterID, []string{"--admin"}, "admin SQL execution")

	branchName := "tdc-e2e-branch-" + suffix
	branchID := ""
	branchDeleted := false
	defer func() {
		if branchID == "" || branchDeleted {
			return
		}
		cleanup := runTDC(
			t,
			bin,
			"--profile", profileName,
			"db", "delete-db-cluster-branch",
			"--db-cluster-id", clusterID,
			"--db-cluster-branch-id", branchID,
			"--confirm-db-cluster-branch-name", branchName,
		)
		if cleanup.exitCode != 0 && cleanup.exitCode != 5 {
			t.Logf("cleanup delete failed for branch %s: exit=%d stdout=%s stderr=%s", branchID, cleanup.exitCode, cleanup.stdout, cleanup.stderr)
		}
	}()

	branchCreate := runTDC(
		t,
		bin,
		"--profile", profileName,
		"db", "create-db-cluster-branch",
		"--db-cluster-id", clusterID,
		"--db-cluster-branch-name", branchName,
	)
	branchCreate.wantExitCode(0)
	createdBranch := decodeLiveBranch(t, branchCreate)
	if createdBranch.ID == "" || createdBranch.DisplayName != branchName {
		t.Fatalf("unexpected created branch: %#v\n%s", createdBranch, branchCreate.stdout)
	}
	branchID = createdBranch.ID

	waitLiveBranch(t, bin, profileName, clusterID, branchID, func(branch liveBranch) bool {
		return branch.ID == branchID && branch.DisplayName == branchName && branch.State == "ACTIVE"
	}, 5*time.Minute, "become ACTIVE after create")

	branches := runTDC(t, bin, "--profile", profileName, "db", "list-db-cluster-branches", "--db-cluster-id", clusterID, "--page-size", "100")
	branches.wantExitCode(0)
	branches.wantStdoutContains(`"branches"`)
	branches.wantStdoutContains(branchID)

	branchQuery := runTDC(t, bin, "--profile", profileName, "db", "list-db-cluster-branches", "--db-cluster-id", clusterID, "--query", "branches[].id")
	branchQuery.wantExitCode(0)
	branchQuery.wantStdoutContains(branchID)

	branchHuman := runTDC(t, bin, "--profile", profileName, "db", "list-db-cluster-branches", "--db-cluster-id", clusterID, "--output", "human")
	branchHuman.wantExitCode(0)
	branchHuman.wantStdoutContains("ID")
	branchHuman.wantStdoutContains(branchName)

	branchDescribe := runTDC(t, bin, "--profile", profileName, "db", "describe-db-cluster-branch", "--db-cluster-id", clusterID, "--db-cluster-branch-id", branchID, "--view", "FULL")
	branchDescribe.wantExitCode(0)
	describedBranch := decodeLiveBranch(t, branchDescribe)
	if describedBranch.ID != branchID || describedBranch.DisplayName != branchName {
		t.Fatalf("unexpected described branch: %#v\n%s", describedBranch, branchDescribe.stdout)
	}

	branchDelete := runTDC(
		t,
		bin,
		"--profile", profileName,
		"db", "delete-db-cluster-branch",
		"--db-cluster-id", clusterID,
		"--db-cluster-branch-id", branchID,
		"--confirm-db-cluster-branch-name", branchName,
	)
	branchDelete.wantExitCode(0)
	deletedBranch := decodeLiveBranch(t, branchDelete)
	if deletedBranch.ID != branchID {
		t.Fatalf("delete response did not reference created branch %s:\n%s", branchID, branchDelete.stdout)
	}
	branchDeleted = true

	waitLiveBranchDeleted(t, bin, profileName, clusterID, branchID, 5*time.Minute)

	update := runTDC(
		t,
		bin,
		"--profile", profileName,
		"db", "update-db-cluster",
		"--db-cluster-id", clusterID,
		"--db-cluster-name", updatedName,
	)
	update.wantExitCode(0)
	updated := decodeLiveCluster(t, update)
	if updated.ID != clusterID || updated.DisplayName != updatedName {
		t.Fatalf("unexpected updated cluster: %#v\n%s", updated, update.stdout)
	}
	currentName = updatedName

	waitLiveCluster(t, bin, profileName, clusterID, func(cluster liveCluster) bool {
		return cluster.ID == clusterID && cluster.DisplayName == updatedName
	}, 3*time.Minute, "show updated display name")

	remove := runTDC(
		t,
		bin,
		"--profile", profileName,
		"db", "delete-db-cluster",
		"--db-cluster-id", clusterID,
		"--confirm-db-cluster-name", updatedName,
	)
	remove.wantExitCode(0)
	removed := decodeLiveCluster(t, remove)
	if removed.ID != clusterID {
		t.Fatalf("delete response did not reference created cluster %s:\n%s", clusterID, remove.stdout)
	}
	deleted = true

	waitLiveClusterDeleted(t, bin, profileName, clusterID, 12*time.Minute)
}

func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv("TDC_LIVE") != "1" {
		t.Skip("TDC_LIVE=1 is required; run make live-e2e")
	}
}

func liveProjectID(t *testing.T, bin, profileName string) string {
	t.Helper()
	projects := runTDC(t, bin, "--profile", profileName, "organization", "list-projects", "--page-size", "1")
	projects.wantExitCode(0)
	var projectList struct {
		Projects []struct {
			ID string `json:"id"`
		} `json:"projects"`
	}
	if err := json.Unmarshal([]byte(projects.stdout), &projectList); err != nil {
		t.Fatalf("decode live projects: %v\n%s", err, projects.stdout)
	}
	if len(projectList.Projects) == 0 || projectList.Projects[0].ID == "" {
		t.Fatalf("live profile %q cannot see a project:\n%s", profileName, projects.stdout)
	}
	return projectList.Projects[0].ID
}

type liveCluster struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	State       string `json:"state"`
	ClusterPlan string `json:"cluster_plan"`
}

type liveBranch struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	ClusterID   string `json:"cluster_id"`
	State       string `json:"state"`
}

type liveVaultToken struct {
	Token   string `json:"token"`
	TokenID string `json:"token_id"`
	GrantID string `json:"grant_id"`
}

type liveGitWorkspace struct {
	WorkspaceID string `json:"workspace_id"`
	RootPath    string `json:"root_path"`
}

type liveGitObjectPack struct {
	PackID string `json:"pack_id"`
}

func decodeLiveCluster(t *testing.T, result commandResult) liveCluster {
	t.Helper()
	var cluster liveCluster
	if err := json.Unmarshal([]byte(result.stdout), &cluster); err != nil {
		t.Fatalf("decode cluster output: %v\n%s", err, result.stdout)
	}
	return cluster
}

func decodeLiveBranch(t *testing.T, result commandResult) liveBranch {
	t.Helper()
	var branch liveBranch
	if err := json.Unmarshal([]byte(result.stdout), &branch); err != nil {
		t.Fatalf("decode branch output: %v\n%s", err, result.stdout)
	}
	return branch
}

func decodeLiveVaultToken(t *testing.T, result commandResult) liveVaultToken {
	t.Helper()
	var token liveVaultToken
	if err := json.Unmarshal([]byte(result.stdout), &token); err != nil {
		t.Fatalf("decode vault token output: %v\n%s", err, result.stdout)
	}
	return token
}

func decodeLiveGitWorkspace(t *testing.T, result commandResult) liveGitWorkspace {
	t.Helper()
	var workspace liveGitWorkspace
	if err := json.Unmarshal([]byte(result.stdout), &workspace); err != nil {
		t.Fatalf("decode git workspace output: %v\n%s", err, result.stdout)
	}
	return workspace
}

func decodeLiveGitObjectPack(t *testing.T, result commandResult) liveGitObjectPack {
	t.Helper()
	var pack liveGitObjectPack
	if err := json.Unmarshal([]byte(result.stdout), &pack); err != nil {
		t.Fatalf("decode git object pack output: %v\n%s", err, result.stdout)
	}
	return pack
}

func waitLiveCluster(t *testing.T, bin, profileName, clusterID string, ready func(liveCluster) bool, timeout time.Duration, description string) liveCluster {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last liveCluster
	for {
		describe := runTDC(t, bin, "--profile", profileName, "db", "describe-db-cluster", "--db-cluster-id", clusterID, "--view", "FULL")
		describe.wantExitCode(0)
		last = decodeLiveCluster(t, describe)
		if ready(last) {
			return last
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for cluster %s to %s; last=%#v", clusterID, description, last)
		}
		time.Sleep(10 * time.Second)
	}
}

func waitLiveBranch(t *testing.T, bin, profileName, clusterID, branchID string, ready func(liveBranch) bool, timeout time.Duration, description string) liveBranch {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last liveBranch
	for {
		describe := runTDC(t, bin, "--profile", profileName, "db", "describe-db-cluster-branch", "--db-cluster-id", clusterID, "--db-cluster-branch-id", branchID, "--view", "FULL")
		describe.wantExitCode(0)
		last = decodeLiveBranch(t, describe)
		if ready(last) {
			return last
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for branch %s to %s; last=%#v", branchID, description, last)
		}
		time.Sleep(5 * time.Second)
	}
}

func waitLiveClusterDeleted(t *testing.T, bin, profileName, clusterID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		describe := runTDC(t, bin, "--profile", profileName, "db", "describe-db-cluster", "--db-cluster-id", clusterID)
		switch describe.exitCode {
		case 0:
			cluster := decodeLiveCluster(t, describe)
			if cluster.ID != clusterID {
				t.Fatalf("post-delete read returned a different cluster: %#v", cluster)
			}
			if cluster.State == "DELETED" {
				return
			}
		case 5:
			return
		case 4:
			// TiDB Cloud can return 403 for a just-deleted cluster that was
			// readable before the successful DELETE.
			return
		default:
			describe.fail("post-delete read should return deleted cluster state, not found, or no longer readable; got exit code %d", describe.exitCode)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for cluster %s to be deleted", clusterID)
		}
		time.Sleep(10 * time.Second)
	}
}

func waitLiveSQL(t *testing.T, bin, profileName, clusterID string, modeArgs []string, description string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Minute)
	var last commandResult
	for {
		args := []string{"--profile", profileName, "db", "execute-sql-statement", "--db-cluster-id", clusterID}
		args = append(args, modeArgs...)
		args = append(args, "--sql", "select 1")
		last = runTDC(t, bin, args...)
		if last.exitCode == 0 {
			last.wantStdoutContains(`"transport": "http"`)
			last.wantStdoutContains(`"row_count": 1`)
			return
		}
		if time.Now().After(deadline) {
			last.fail("timed out waiting for %s; got exit code %d", description, last.exitCode)
		}
		time.Sleep(10 * time.Second)
	}
}

func waitLiveFSResult(t *testing.T, bin string, args []string, want string, timeout time.Duration, description string) commandResult {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last commandResult
	for {
		last = runTDC(t, bin, args...)
		if last.exitCode == 0 && strings.Contains(last.stdout, want) {
			return last
		}
		if time.Now().After(deadline) {
			last.fail("timed out waiting for tdc fs to %s", description)
		}
		time.Sleep(5 * time.Second)
	}
}

func waitLiveLocalFile(t *testing.T, path, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		data, err := os.ReadFile(path)
		if err == nil && string(data) == want {
			return
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("content mismatch: got %q", data)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for mounted file %s: %v", path, lastErr)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func waitLiveRemoteRead(t *testing.T, bin, profileName, remotePath, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last commandResult
	for {
		last = runTDC(t, bin, "--profile", profileName, "fs", "read-file", "--path", remotePath)
		if last.exitCode == 0 && last.stdout == want {
			return
		}
		if time.Now().After(deadline) {
			last.fail("timed out waiting for remote file %s to match mounted write", remotePath)
		}
		time.Sleep(1 * time.Second)
	}
}

func waitLiveBranchDeleted(t *testing.T, bin, profileName, clusterID, branchID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		describe := runTDC(t, bin, "--profile", profileName, "db", "describe-db-cluster-branch", "--db-cluster-id", clusterID, "--db-cluster-branch-id", branchID)
		switch describe.exitCode {
		case 0:
			branch := decodeLiveBranch(t, describe)
			if branch.ID != branchID {
				t.Fatalf("post-delete read returned a different branch: %#v", branch)
			}
			if branch.State == "DELETED" {
				return
			}
		case 5:
			return
		case 4:
			return
		default:
			describe.fail("post-delete branch read should return deleted branch state, not found, or no longer readable; got exit code %d", describe.exitCode)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for branch %s to be deleted", branchID)
		}
		time.Sleep(5 * time.Second)
	}
}

func cleanupLiveSQLCredentials(t *testing.T, clusterID string) {
	t.Helper()
	if clusterID == "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Logf("cannot determine home directory for SQL credential cleanup: %v", err)
		return
	}
	path := filepath.Join(home, ".tdc", "db_users", clusterID)
	if err := os.RemoveAll(path); err != nil {
		t.Logf("cleanup SQL credentials failed for %s: %v", path, err)
	}
}

func liveProfileName(t *testing.T) string {
	t.Helper()
	profileName := os.Getenv("TDC_PROFILE")
	if profileName == "" {
		profileName = defaultLiveProfile
	}
	if profileName != defaultLiveProfile {
		t.Fatalf("live e2e must use profile %q, got %q", defaultLiveProfile, profileName)
	}
	return profileName
}

func liveProfile(t *testing.T) *config.Profile {
	t.Helper()
	profileName := liveProfileName(t)
	profile, err := auth.LoadProfile(context.Background(), config.LoadOptions{
		Profile:         profileName,
		ProfileExplicit: true,
	})
	if err != nil {
		t.Fatalf("load live e2e profile %q: %v\nconfigure it with: bin/tdc configure --profile %s", profileName, err, profileName)
	}
	return profile
}

func liveDigestClient(t *testing.T, profile *config.Profile, endpoint endpoints.Endpoint, permission authz.Permission) *api.Client {
	t.Helper()
	client, err := api.NewDigestClient(profile, endpoint, permission, api.Options{
		Timeout:    30 * time.Second,
		MaxRetries: 1,
		UserAgent:  "tdc-live-e2e",
	})
	if err != nil {
		t.Fatalf("create live API client for %s: %v", endpoint.Service, err)
	}
	return client
}

func liveFSClient(t *testing.T, profile *config.Profile, permission authz.Permission) *apifs.Client {
	t.Helper()
	endpoint, err := endpoints.NewResolver().ResolveFS(profile.CloudProvider, profile.RegionCode)
	if err != nil {
		t.Fatalf("resolve live tdc fs endpoint: %v", err)
	}
	client, err := api.NewBearerClient(profile.Name, profile.FSAPIKey, endpoint, permission, api.Options{
		Timeout:    45 * time.Second,
		MaxRetries: 1,
		UserAgent:  "tdc-live-e2e",
	})
	if err != nil {
		t.Fatalf("create live tdc fs client: %v", err)
	}
	return apifs.New(client)
}

func initiateLiveUpload(t *testing.T, client *apifs.Client, remotePath, localPath string) string {
	t.Helper()
	file, err := os.Open(localPath)
	if err != nil {
		t.Fatalf("open resume upload source: %v", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatalf("stat resume upload source: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	plan, err := client.InitiateUploadFromReader(ctx, remotePath, file, info.Size(), apifs.UploadFileOptions{})
	if err != nil {
		t.Fatalf("initiate live upload for resume: %v", err)
	}
	if plan.UploadID == "" {
		t.Fatalf("live upload plan missing upload id: %#v", plan)
	}
	return plan.UploadID
}

func liveGETJSON(t *testing.T, client *api.Client, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	req, err := client.NewRequest(ctx, "GET", path, nil)
	if err != nil {
		t.Fatalf("build live request %s: %v", path, err)
	}

	var payload any
	if err := client.DoJSON(req, &payload); err != nil {
		t.Fatalf("live GET %s failed: %v", path, err)
	}
	if payload == nil {
		t.Fatalf("live GET %s returned empty JSON payload", path)
	}
	switch typed := payload.(type) {
	case map[string]any:
		if len(typed) == 0 {
			t.Fatalf("live GET %s returned empty JSON object", path)
		}
	case []any:
		if len(typed) == 0 {
			t.Fatalf("live GET %s returned empty JSON array", path)
		}
	default:
		if strings.TrimSpace(path) == "" {
			t.Fatalf("live GET returned unexpected scalar payload for empty path")
		}
	}
}
