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
	"sync"
	"testing"
	"time"

	"github.com/tidbcloud/tdc/internal/api"
	"github.com/tidbcloud/tdc/internal/api/endpoints"
	apifs "github.com/tidbcloud/tdc/internal/api/fs"
	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/auth"
	"github.com/tidbcloud/tdc/internal/authz"
	"github.com/tidbcloud/tdc/internal/config"
	"github.com/tidbcloud/tdc/internal/fs/fscred"
)

const defaultLiveProfile = "live-e2e"

var (
	liveFSResourceMu          sync.Mutex
	liveFSResourceAutoCreated bool
	liveProfileConfigureMu    sync.Mutex
)

func TestMain(m *testing.M) {
	code := m.Run()
	if liveFSResourceAutoCreated {
		cleanupAutoCreatedLiveFSResource()
	}
	os.Exit(code)
}

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

func TestLiveFSResourceRegistryLifecycle(t *testing.T) {
	requireLive(t)

	bin := tdcBinary(t)
	profileName := liveProfileName(t)
	profile := liveProfile(t)
	originalDefault := profile.FSDefaultFileSystemName
	suffix := fmt.Sprintf("%s-%d", time.Now().UTC().Format("20060102150405"), os.Getpid())
	names := []string{"tdc-e2e-fs-" + suffix + "-a", "tdc-e2e-fs-" + suffix + "-b"}
	created := make(map[string]bool, len(names))
	stateMutated := false
	defer func() {
		for i := len(names) - 1; i >= 0; i-- {
			name := names[i]
			if !created[name] {
				continue
			}
			result := runTDC(t, bin, "--profile", profileName, "fs", "delete-file-system", "--file-system-name", name, "--confirm-file-system-name", name)
			if result.exitCode != 0 {
				t.Logf("cleanup delete failed for tdc fs resource %q: exit=%d stdout=%s stderr=%s", name, result.exitCode, result.stdout, result.stderr)
			}
		}
		if stateMutated {
			restoreLiveFSDefault(t, bin, profileName, originalDefault)
		}
	}()

	for i, name := range names {
		create := runTDC(t, bin, "--profile", profileName, "fs", "create-file-system", "--file-system-name", name)
		if create.exitCode != 0 {
			if isLiveFSQuotaError(create.stderr) {
				if i == 0 {
					t.Skipf("tdc fs live registry lifecycle requires one free Starter slot: %s", strings.TrimSpace(create.stderr))
				}
				t.Logf("second tdc fs resource could not be created because Starter quota is full; single-resource live flow completed and multi-resource selection remains covered by the fake-companion e2e: %s", strings.TrimSpace(create.stderr))
				check := runTDC(t, bin, "--profile", profileName, "fs", "check-file-system", "--file-system-name", names[0])
				check.wantExitCode(0)
				check.wantStdoutContains(`"status": "passed"`)
				return
			}
			create.fail("create live tdc fs registry resource")
		}
		if strings.Contains(create.stdout, `"status": "exists"`) {
			create.fail("generated live tdc fs resource name already existed; refusing to delete a resource not created by this test")
		}
		created[name] = true
		stateMutated = true
		create.wantStdoutContains(`"credentials_stored": true`)
		selected := resolveLiveFSResource(t, profile, name)
		waitLiveFSReady(t, bin, profileName, selected, 10*time.Minute)
	}

	list := runTDC(t, bin, "--profile", profileName, "fs", "list-file-systems")
	list.wantExitCode(0)
	for _, name := range names {
		list.wantStdoutContains(`"file_system_name": "` + name + `"`)
	}

	setDefault := runTDC(t, bin, "--profile", profileName, "fs", "set-default-file-system", "--file-system-name", names[0])
	setDefault.wantExitCode(0)
	defaultCheck := runTDC(t, bin, "--profile", profileName, "fs", "check-file-system")
	defaultCheck.wantExitCode(0)
	defaultCheck.wantStdoutContains(`"file_system_name": "` + names[0] + `"`)
	explicitCheck := runTDC(t, bin, "--profile", profileName, "fs", "check-file-system", "--file-system-name", names[1])
	explicitCheck.wantExitCode(0)
	explicitCheck.wantStdoutContains(`"file_system_name": "` + names[1] + `"`)

	deleteFirst := runTDC(t, bin, "--profile", profileName, "fs", "delete-file-system", "--file-system-name", names[0], "--confirm-file-system-name", names[0])
	deleteFirst.wantExitCode(0)
	created[names[0]] = false
	remaining := runTDC(t, bin, "--profile", profileName, "fs", "describe-file-system", "--file-system-name", names[1])
	remaining.wantExitCode(0)
	remaining.wantStdoutContains(`"file_system_name": "` + names[1] + `"`)

	deleteSecond := runTDC(t, bin, "--profile", profileName, "fs", "delete-file-system", "--file-system-name", names[1], "--confirm-file-system-name", names[1])
	deleteSecond.wantExitCode(0)
	created[names[1]] = false
}

func TestLiveCurrentCommandSurface(t *testing.T) {
	requireLive(t)

	bin := tdcBinary(t)
	profileName := liveProfileName(t)
	fileSystemName := liveFileSystemName(t)
	ensureLiveFSResource(t, bin, profileName)

	helpCommands := [][]string{
		{"help"},
		{"update", "help"},
		{"db", "help"},
		{"fs", "help"},
		{"fs-git", "help"},
		{"fs-journal", "help"},
		{"fs-vault", "help"},
		{"organization", "help"},
		{"db", "create-db-cluster", "help"},
		{"db", "list-db-clusters", "help"},
		{"fs", "create-file-system", "help"},
		{"fs", "list-file-systems", "help"},
		{"fs", "describe-file-system", "help"},
		{"fs", "set-default-file-system", "help"},
		{"fs", "unset-default-file-system", "help"},
		{"fs", "copy-file", "help"},
		{"fs", "read-file", "help"},
		{"fs", "chmod-file", "help"},
		{"fs", "create-symlink", "help"},
		{"fs", "create-hardlink", "help"},
		{"fs", "create-layer", "help"},
		{"fs", "list-layers", "help"},
		{"fs", "describe-layer", "help"},
		{"fs", "diff-layer", "help"},
		{"fs", "create-layer-checkpoint", "help"},
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
		{"fs-vault", "create-secret", "help"},
		{"fs-vault", "replace-secret", "help"},
		{"fs-vault", "read-secret", "help"},
		{"fs-vault", "list-secrets", "help"},
		{"fs-vault", "delete-secret", "help"},
		{"fs-vault", "create-grant", "help"},
		{"fs-vault", "delete-grant", "help"},
		{"fs-vault", "list-audit-events", "help"},
		{"fs-vault", "run-with-secret", "help"},
		{"fs-vault", "mount-vault", "help"},
		{"fs-vault", "unmount-vault", "help"},
		{"fs-journal", "create-journal", "help"},
		{"fs-journal", "append-journal-entries", "help"},
		{"fs-journal", "read-journal-entries", "help"},
		{"fs-journal", "search-journal-entries", "help"},
		{"fs-journal", "verify-journal", "help"},
		{"fs-git", "clone-git-workspace", "help"},
		{"fs-git", "hydrate-git-workspace", "help"},
		{"fs-git", "add-git-worktree", "help"},
		{"fs-git", "remove-git-worktree", "help"},
	}
	for _, args := range helpCommands {
		result := runTDC(t, bin, args...)
		result.wantExitCode(0)
		result.wantStdoutContains("Usage:")
	}

	mutatingDryRunCommands := [][]string{
		{"db", "create-db-cluster-branch", "--db-cluster-id", "cluster-1", "--db-cluster-branch-name", "dev"},
		{"db", "delete-db-cluster-branch", "--db-cluster-id", "cluster-1", "--db-cluster-branch-id", "branch-1"},
		{"db", "create-db-sql-users", "--db-cluster-id", "cluster-1"},
		{"fs", "create-file-system", "--file-system-name", fileSystemName},
		{"fs", "delete-file-system", "--file-system-name", fileSystemName, "--confirm-file-system-name", fileSystemName},
		{"fs", "create-layer", "--layer-id", "layer-1", "--base-root-path", "/workspace", "--layer-name", "dev"},
		{"fs", "create-layer-checkpoint", "--layer-id", "layer-1", "--checkpoint-id", "cp-1"},
		{"fs", "rollback-layer", "--layer-id", "layer-1"},
		{"fs", "commit-layer", "--layer-id", "layer-1"},
		{"fs", "pack-file-system", "--local-root", "/tmp/tdc-e2e-pack", "--remote-root", "/workspace", "--mount-profile", "portable"},
		{"fs", "unpack-file-system", "--local-root", "/tmp/tdc-e2e-pack", "--remote-root", "/workspace", "--mount-profile", "portable"},
		{"fs", "mount-file-system", "--mount-path", "/tmp/tdc-e2e-mount", "--driver", "webdav"},
		{"fs", "unmount-file-system", "--mount-path", "/tmp/tdc-e2e-mount"},
		{"fs-vault", "create-secret", "--secret-name", "tdc-e2e-secret", "--field", "DB_URL=mysql://example"},
		{"fs-vault", "replace-secret", "--secret-path", "/n/vault/tdc-e2e-secret", "--from-directory", "/tmp"},
		{"fs-vault", "delete-secret", "--secret-name", "tdc-e2e-secret"},
		{"fs-vault", "create-grant", "--agent-id", "tdc-live-e2e", "--scope", "tdc-e2e-secret/DB_URL", "--permission", "read", "--ttl", "10m"},
		{"fs-vault", "delete-grant", "--grant-id", "grant-1"},
		{"fs-vault", "mount-vault", "--mount-path", "/tmp/tdc-e2e-vault"},
		{"fs-vault", "unmount-vault", "--mount-path", "/tmp/tdc-e2e-vault"},
		{"fs-journal", "create-journal", "--journal-id", "jrn-tdc-e2e", "--journal-kind", "agent"},
		{"fs-journal", "append-journal-entries", "--journal-id", "jrn-tdc-e2e", "--entry-json", `{"type":"task.started"}`},
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
	for _, args := range [][]string{
		{"fs", "set-default-file-system", "--file-system-name", fileSystemName},
		{"fs", "unset-default-file-system"},
	} {
		fullArgs := append([]string{"--profile", profileName}, args...)
		fullArgs = append(fullArgs, "--dry-run")
		result := runTDC(t, bin, fullArgs...)
		result.wantExitCode(0)
		result.wantStdoutContains(`"local_resource_registry"`)
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
		{"db", "format-db-connection-string"},
		{"db", "execute-sql-statement"},
		{"fs", "check-file-system"},
		{"fs", "list-file-systems"},
		{"fs", "describe-file-system"},
		{"fs", "read-file"},
		{"fs", "list-files"},
		{"fs", "describe-file"},
		{"fs", "search-file-content"},
		{"fs", "find-files"},
		{"fs", "list-layers"},
		{"fs", "describe-layer"},
		{"fs", "diff-layer"},
		{"fs-vault", "read-secret"},
		{"fs-vault", "list-secrets"},
		{"fs-vault", "list-audit-events"},
		{"fs-vault", "run-with-secret"},
		{"fs-journal", "read-journal-entries"},
		{"fs-journal", "search-journal-entries"},
		{"fs-journal", "verify-journal"},
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
			Type string `json:"type"`
		} `json:"projects"`
		NextPageToken string `json:"next_page_token"`
	}
	if err := json.Unmarshal([]byte(projects.stdout), &projectList); err != nil {
		t.Fatalf("decode organization list-projects output: %v\n%s", err, projects.stdout)
	}
	if len(projectList.Projects) == 0 || projectList.Projects[0].ID == "" || projectList.Projects[0].Type == "" {
		t.Fatalf("expected live profile %q to see at least one project with an id and type:\n%s", profileName, projects.stdout)
	}

	query := runTDC(t, bin, "--profile", profileName, "organization", "list-projects", "--page-size", "1", "--query", "projects[0].id")
	query.wantExitCode(0)
	query.wantStdoutContains(projectList.Projects[0].ID)

	text := runTDC(t, bin, "--profile", profileName, "organization", "list-projects", "--page-size", "1", "--output", "text")
	text.wantExitCode(0)
	text.wantStdoutContains("ID")
	text.wantStdoutContains("TYPE")
	text.wantStdoutContains(projectList.Projects[0].ID)
	text.wantStdoutContains(projectList.Projects[0].Type)

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

	clusterText := runTDC(t, bin, "--profile", profileName, "db", "list-db-clusters", "--page-size", "1", "--output", "text")
	clusterText.wantExitCode(0)
	clusterText.wantStdoutContains("ID")

	if len(clusterList.Clusters) > 0 && clusterList.Clusters[0].ID != "" {
		describe := runTDC(t, bin, "--profile", profileName, "db", "describe-db-cluster", "--db-cluster-id", clusterList.Clusters[0].ID)
		describe.wantExitCode(0)
		describe.wantStdoutContains(`"id"`)
		describe.wantStdoutContains(clusterList.Clusters[0].ID)
	}

	checkUpdateHelp := runTDC(t, bin, "update", "help")
	checkUpdateHelp.wantExitCode(0)
	checkUpdateHelp.wantStdoutContains("--check")
	checkUpdateHelp.wantStdoutContains("--fail-if-update-available")

	checkUpdateHelp.wantStdoutContains("--yes")
}

func resolveLiveFSResource(t *testing.T, profile *config.Profile, name string) *config.Profile {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("determine home directory: %v", err)
	}
	selected, _, err := fscred.Resolve(home, profile, name, true, nil)
	if err != nil {
		t.Fatalf("resolve live tdc fs resource %q: %v", name, err)
	}
	return selected
}

func isLiveFSQuotaError(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "maximum number of free clusters") ||
		strings.Contains(message, "quota or capacity limit")
}

func restoreLiveFSDefault(t *testing.T, bin, profileName, originalDefault string) {
	t.Helper()
	var result commandResult
	if originalDefault == "" {
		result = runTDC(t, bin, "--profile", profileName, "fs", "unset-default-file-system")
	} else {
		result = runTDC(t, bin, "--profile", profileName, "fs", "set-default-file-system", "--file-system-name", originalDefault)
	}
	if result.exitCode != 0 {
		t.Logf("restore tdc fs default failed for profile %q: exit=%d stdout=%s stderr=%s", profileName, result.exitCode, result.stdout, result.stderr)
	}
}

func TestLiveFSDataPlaneLifecycle(t *testing.T) {
	requireLive(t)

	bin := tdcBinary(t)
	profileName := liveProfileName(t)
	profile := ensureLiveFSResource(t, bin, profileName)
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

	list := runTDC(t, bin, "--profile", profileName, "fs", "list-files", "--path", rootPath)
	list.wantExitCode(0)
	list.wantStdoutContains("README.md")

	listText := runTDC(t, bin, "--profile", profileName, "fs", "list-files", "--path", rootPath, "--output", "text")
	listText.wantExitCode(0)
	listText.wantStdoutContains("NAME")
	listText.wantStdoutContains("README.md")

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

	waitLiveFSResult(t, bin, []string{"--profile", profileName, "fs", "grep", "--path", aliasDir, "--pattern", "alias live e2e", "--limit", "5"}, "alias.txt", 5*time.Minute, "grep alias file content")
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

	layerCopyPath := rootPath + "/copy-layer.txt"
	layerCopyContent := "copy-file into layer " + suffix + "\n"
	layerCopyFile := filepath.Join(t.TempDir(), "copy-layer.txt")
	if err := os.WriteFile(layerCopyFile, []byte(layerCopyContent), 0o644); err != nil {
		t.Fatalf("write layer copy file: %v", err)
	}
	copyToLayer := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-local", layerCopyFile, "--to-remote", layerCopyPath, "--layer-id", layerID)
	copyToLayer.wantExitCode(0)
	copyToLayer.wantStdoutContains(`"status"`)

	diffLayer := runTDC(t, bin, "--profile", profileName, "fs", "diff-layer", "--layer-id", layerID, "--output", "text")
	diffLayer.wantExitCode(0)
	diffLayer.wantStdoutContains(layerCopyPath)

	createCheckpoint := runTDC(t, bin, "--profile", profileName, "fs", "create-layer-checkpoint", "--layer-id", layerID, "--checkpoint-id", checkpointID, "--label", "live-e2e")
	createCheckpoint.wantExitCode(0)
	createCheckpoint.wantStdoutContains(checkpointID)

	waitLiveFSResult(t, bin, []string{"--profile", profileName, "fs", "find-files", "--path", rootPath, "--file-name-pattern", "copy-layer.txt", "--layer-id", layerID, "--limit", "5"}, layerCopyPath, 2*time.Minute, "find file inside layer")

	commitLayer := runTDC(t, bin, "--profile", profileName, "fs", "commit-layer", "--layer-id", layerID)
	commitLayer.wantExitCode(0)
	commitLayer.wantStdoutContains(`"status"`)
	layerClosed = true
	waitLiveRemoteRead(t, bin, profileName, layerCopyPath, layerCopyContent, 30*time.Second)

	vaultSecretName := "tdc-e2e-vault-" + suffix
	vaultDeleted := false
	defer func() {
		if vaultDeleted {
			return
		}
		cleanup := runTDC(t, bin, "--profile", profileName, "fs-vault", "delete-secret", "--secret-name", vaultSecretName)
		if cleanup.exitCode != 0 && cleanup.exitCode != 5 {
			t.Logf("cleanup vault secret failed for %s: exit=%d stdout=%s stderr=%s", vaultSecretName, cleanup.exitCode, cleanup.stdout, cleanup.stderr)
		}
	}()
	createVaultSecret := runTDC(
		t,
		bin,
		"--profile", profileName,
		"fs-vault", "create-secret",
		"--secret-name", vaultSecretName,
		"--field", "DB_URL=mysql://"+suffix,
		"--field", "PASSWORD=secret-"+suffix,
	)
	createVaultSecret.wantExitCode(0)
	createVaultSecret.wantStdoutContains(vaultSecretName)

	listVaultSecrets := runTDC(t, bin, "--profile", profileName, "fs-vault", "list-secrets")
	listVaultSecrets.wantExitCode(0)
	listVaultSecrets.wantStdoutContains(vaultSecretName)

	readVaultSecret := runTDC(t, bin, "--profile", profileName, "fs-vault", "read-secret", "--secret-name", vaultSecretName, "--field", "PASSWORD", "--format", "raw")
	readVaultSecret.wantExitCode(0)
	if readVaultSecret.stdout != "secret-"+suffix {
		readVaultSecret.fail("vault read-secret --format raw should return exact field bytes")
	}

	readVaultEnv := runTDC(t, bin, "--profile", profileName, "fs-vault", "read-secret", "--secret-name", vaultSecretName, "--format", "env")
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
	replaceVaultSecret := runTDC(t, bin, "--profile", profileName, "fs-vault", "replace-secret", "--secret-path", "/n/vault/"+vaultSecretName, "--from-directory", replaceVaultDir)
	replaceVaultSecret.wantExitCode(0)
	replaceVaultSecret.wantStdoutContains(vaultSecretName)

	readReplacedVaultSecret := runTDC(t, bin, "--profile", profileName, "fs-vault", "read-secret", "--secret-name", vaultSecretName, "--field", "PASSWORD", "--format", "raw")
	readReplacedVaultSecret.wantExitCode(0)
	if readReplacedVaultSecret.stdout != "replaced-"+suffix {
		readReplacedVaultSecret.fail("vault replace-secret should replace stored field bytes")
	}

	createVaultGrant := runTDC(
		t,
		bin,
		"--profile", profileName,
		"fs-vault", "create-grant",
		"--agent-id", "tdc-live-e2e",
		"--scope", vaultSecretName,
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
		cleanup := runTDC(t, bin, "--profile", profileName, "fs-vault", "delete-grant", "--grant-id", vaultGrant.GrantID, "--reason", "cleanup")
		if cleanup.exitCode != 0 && cleanup.exitCode != 5 {
			t.Logf("cleanup vault grant failed for %s: exit=%d stdout=%s stderr=%s", vaultGrant.GrantID, cleanup.exitCode, cleanup.stdout, cleanup.stderr)
		}
	}()
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
			cleanupUnmount := runTDC(t, bin, "--profile", profileName, "fs-vault", "unmount-vault", "--mount-path", vaultMountPath, "--ignore-absent", "--force")
			if cleanupUnmount.exitCode != 0 {
				t.Logf("cleanup vault unmount failed for %s: exit=%d stdout=%s stderr=%s", vaultMountPath, cleanupUnmount.exitCode, cleanupUnmount.stdout, cleanupUnmount.stderr)
			}
		}()
		mountVault := runTDC(t, bin, "--profile", profileName, "fs-vault", "mount-vault", "--mount-path", vaultMountPath, "--vault-token", vaultGrant.Token, "--ready-timeout", "30s")
		mountVault.wantExitCode(0)
		mountVault.wantStdoutContains(`"status": "mounted"`)
		waitLiveLocalFile(t, filepath.Join(vaultMountPath, vaultSecretName, "PASSWORD"), "replaced-"+suffix, 30*time.Second)
		unmountVault := runTDC(t, bin, "--profile", profileName, "fs-vault", "unmount-vault", "--mount-path", vaultMountPath)
		unmountVault.wantExitCode(0)
		unmountVault.wantStdoutContains(`"status": "unmounted"`)
		vaultUnmounted = true
	}
	readVaultWithGrant := runTDC(t, bin, "--profile", profileName, "fs-vault", "read-secret", "--secret-name", vaultSecretName, "--field", "DB_URL", "--format", "raw", "--vault-token", vaultGrant.Token)
	readVaultWithGrant.wantExitCode(0)
	if readVaultWithGrant.stdout != "mysql://replaced-"+suffix {
		readVaultWithGrant.fail("delegated vault grant should read scoped field")
	}
	deleteVaultGrant := runTDC(t, bin, "--profile", profileName, "fs-vault", "delete-grant", "--grant-id", vaultGrant.GrantID, "--reason", "live-e2e-complete")
	deleteVaultGrant.wantExitCode(0)
	grantDeleted = true

	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath("env"); err == nil {
			runWithVault := runTDC(t, bin, "--profile", profileName, "fs-vault", "run-with-secret", "--secret-path", "/n/vault/"+vaultSecretName, "--", "env")
			runWithVault.wantExitCode(0)
			runWithVault.wantStdoutContains("DB_URL=mysql://replaced-" + suffix)
			runWithVault.wantStdoutContains("PASSWORD=replaced-" + suffix)
			runWithVault.wantStdoutNotContains("TDC_PRIVATE_KEY=")
		}
	}

	listVaultAuditEvents := runTDC(t, bin, "--profile", profileName, "fs-vault", "list-audit-events", "--secret-name", vaultSecretName, "--limit", "20")
	listVaultAuditEvents.wantExitCode(0)
	listVaultAuditEvents.wantStdoutContains(`"events"`)

	deleteVaultSecret := runTDC(t, bin, "--profile", profileName, "fs-vault", "delete-secret", "--secret-name", vaultSecretName)
	deleteVaultSecret.wantExitCode(0)
	deleteVaultSecret.wantStdoutContains(`"status": "deleted"`)
	vaultDeleted = true

	journalID := "jrn-tdc-e2e-" + suffix
	appendID := "app-tdc-e2e-" + suffix
	createJournal := runTDC(
		t,
		bin,
		"--profile", profileName,
		"fs-journal", "create-journal",
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
		"fs-journal", "append-journal-entries",
		"--journal-id", journalID,
		"--idempotency-key", appendID,
		"--entry-json", `{"type":"task.started","summary":{"message":"tdc live e2e `+suffix+`"}}`,
		"--subject", "path:"+rootPath,
	)
	appendJournal.wantExitCode(0)
	appendJournal.wantStdoutContains(`"count": 1`)
	appendJournal.wantStdoutContains(appendID)

	readJournal := runTDC(t, bin, "--profile", profileName, "fs-journal", "read-journal-entries", "--journal-id", journalID, "--limit", "10")
	readJournal.wantExitCode(0)
	readJournal.wantStdoutContains(journalID)
	readJournal.wantStdoutContains("task.started")

	waitLiveFSResult(
		t,
		bin,
		[]string{
			"--profile", profileName,
			"fs-journal", "search-journal-entries",
			"--entry-type", "task.started",
			"--label", "test=tdc-e2e",
			"--limit", "10",
		},
		journalID,
		2*time.Minute,
		"index journal entry",
	)

	verifyJournal := runTDC(t, bin, "--profile", profileName, "fs-journal", "verify-journal", "--journal-id", journalID, "--output", "text")
	verifyJournal.wantExitCode(0)
	verifyJournal.wantStdoutContains("ok journal=" + journalID)

	waitLiveFSResult(t, bin, []string{"--profile", profileName, "fs", "search-file-content", "--path", rootPath, "--pattern", "tdc fs live e2e", "--limit", "5"}, "README.md", 5*time.Minute, "find uploaded file content")
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
	recursiveUpload.wantStdoutContains(`"status": "copied"`)
	readTreeFile := runTDC(t, bin, "--profile", profileName, "fs", "read-file", "--path", treeRoot+"/nested/beta.txt")
	readTreeFile.wantExitCode(0)
	if readTreeFile.stdout != "beta "+suffix {
		readTreeFile.fail("recursive local-to-remote copy should preserve nested file contents")
	}

	treeCopyRoot := rootPath + "/tree-copy"
	recursiveRemoteCopy := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-remote", treeRoot, "--to-remote", treeCopyRoot, "--recursive")
	recursiveRemoteCopy.wantExitCode(0)
	recursiveRemoteCopy.wantStdoutContains(`"status": "copied"`)
	readCopiedTreeFile := runTDC(t, bin, "--profile", profileName, "fs", "read-file", "--path", treeCopyRoot+"/nested/beta.txt")
	readCopiedTreeFile.wantExitCode(0)
	if readCopiedTreeFile.stdout != "beta "+suffix {
		readCopiedTreeFile.fail("recursive remote-to-remote copy should preserve nested file contents")
	}

	downloadTree := filepath.Join(t.TempDir(), "download-tree")
	recursiveDownload := runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--from-remote", treeRoot, "--to-local", downloadTree, "--recursive")
	recursiveDownload.wantExitCode(0)
	recursiveDownload.wantStdoutContains(`"status": "copied"`)
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

	bin := tdcBinary(t)
	profileName := liveProfileName(t)
	ensureLiveFSResource(t, bin, profileName)
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
	upload := runLiveFSSetupCommand(t, bin, "--profile", profileName, "fs", "copy-file", "--from-local", localSeed, "--to-remote", remoteRoot+"/README.md")
	upload.wantExitCode(0)

	mount := runTDC(t, bin, "--profile", profileName, "fs", "mount", "--mount-path", mountPath, "--remote-path", remoteRoot, "--ready-timeout", "30s")
	mount.wantExitCode(0)
	mount.wantStdoutContains(`"status": "mounted"`)
	mount.wantStdoutContains(`"driver":`)

	waitLiveLocalFile(t, filepath.Join(mountPath, "README.md"), seedContent, 30*time.Second)
	overwriteContent := "overwritten through mounted tdc fs " + suffix + "\n"
	if err := os.WriteFile(filepath.Join(mountPath, "README.md"), []byte(overwriteContent), 0o644); err != nil {
		t.Fatalf("overwrite existing remote file through mount failed: %v", err)
	}
	localWrite := "written through mounted tdc fs " + suffix + "\n"
	if err := os.WriteFile(filepath.Join(mountPath, "local-write.txt"), []byte(localWrite), 0o644); err != nil {
		t.Fatalf("write through mount failed: %v", err)
	}
	if strings.Contains(mount.stdout, `"driver": "fuse"`) {
		drain := runTDC(t, bin, "--profile", profileName, "fs", "drain", "--mount-path", mountPath, "--timeout", "30s")
		drain.wantExitCode(0)
		drain.wantStdoutContains(`"status": "drained"`)
	}
	waitLiveRemoteRead(t, bin, profileName, remoteRoot+"/README.md", overwriteContent, 30*time.Second)
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

	bin := tdcBinary(t)
	profileName := liveProfileName(t)
	ensureLiveFSResource(t, bin, profileName)
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
	upload := runLiveFSSetupCommand(t, bin, "--profile", profileName, "fs", "copy-file", "--from-local", localSeed, "--to-remote", remoteRoot+"/README.md")
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
	releaseAutoCreatedLiveFSResource(t, bin, profileName)
	projectID := liveProfile(t).ProjectID

	suffix := time.Now().UTC().Format("20060102150405")
	clusterName := "tdc-e2e-" + suffix
	updatedName := clusterName + "-u"
	var clusterID string
	deleted := false
	defer func() {
		if clusterID == "" || deleted {
			return
		}
		cleanup := runTDC(t, bin, "--profile", profileName, "db", "delete-db-cluster", "--db-cluster-id", clusterID)
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
	if described.Labels["tidb.cloud/project"] != projectID {
		t.Fatalf("cluster project label = %q, want configured default %q: %#v", described.Labels["tidb.cloud/project"], projectID, described)
	}

	prepare := runTDC(t, bin, "--profile", profileName, "db", "create-db-sql-users", "--db-cluster-id", clusterID)
	prepare.wantExitCode(0)
	prepare.wantStdoutContains(`"read_only"`)
	prepare.wantStdoutContains(`"read_write"`)
	prepare.wantStdoutContains(`"admin"`)

	prepareAgain := runTDC(t, bin, "--profile", profileName, "db", "create-db-sql-users", "--db-cluster-id", clusterID)
	prepareAgain.wantExitCode(0)
	prepareAgain.wantStdoutContains(`"exists"`)

	connectionString := runTDC(t, bin, "--profile", profileName, "db", "format-db-connection-string", "--db-cluster-id", clusterID, "--read-write", "--database", "test")
	connectionString.wantExitCode(0)
	connectionString.wantStdoutContains(`"format": "mysql-uri"`)
	connectionString.wantStdoutContains(`"access_mode": "read_write"`)
	connectionString.wantStdoutContains(`"connection_string"`)

	connectionEnv := runTDC(t, bin, "--profile", profileName, "db", "format-db-connection-string", "--db-cluster-id", clusterID, "--read-only", "--format", "env")
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

	branchText := runTDC(t, bin, "--profile", profileName, "db", "list-db-cluster-branches", "--db-cluster-id", clusterID, "--output", "text")
	branchText.wantExitCode(0)
	branchText.wantStdoutContains("ID")
	branchText.wantStdoutContains(branchName)

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
	waitLiveCluster(t, bin, profileName, clusterID, func(cluster liveCluster) bool {
		return cluster.ID == clusterID && cluster.DisplayName == updatedName
	}, 3*time.Minute, "show updated display name")

	remove := runTDC(
		t,
		bin,
		"--profile", profileName,
		"db", "delete-db-cluster",
		"--db-cluster-id", clusterID,
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

type liveCluster struct {
	ID          string            `json:"id"`
	DisplayName string            `json:"display_name"`
	State       string            `json:"state"`
	ClusterPlan string            `json:"cluster_plan"`
	Labels      map[string]string `json:"labels"`
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
			last.wantStdoutContains(`"transport": "https"`)
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

func ensureLiveFSResource(t *testing.T, bin, profileName string) *config.Profile {
	t.Helper()
	liveFSResourceMu.Lock()
	defer liveFSResourceMu.Unlock()

	profile := liveProfile(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("determine home directory: %v", err)
	}
	if err := fscred.MigrateLegacy(home, profile); err != nil {
		t.Fatalf("migrate live fs resource: %v", err)
	}
	name := liveFileSystemName(t)
	if selected, _, err := fscred.Resolve(home, profile, name, true, nil); err == nil {
		waitLiveFSReady(t, bin, profileName, selected, 10*time.Minute)
		return selected
	} else if apperr.CodeFor(err) != "fs.resource_not_found" {
		t.Fatalf("resolve live fs resource %q: %v", name, err)
	}

	create := runTDC(t, bin, "--profile", profileName, "fs", "create-file-system", "--file-system-name", name)
	create.wantExitCode(0)
	create.wantStdoutContains(`"credentials_stored": true`)
	liveFSResourceAutoCreated = true

	profile = liveProfile(t)
	selected, _, err := fscred.Resolve(home, profile, name, true, nil)
	if err != nil {
		t.Fatalf("tdc fs resource %q was created but is not in profile %q registry: %v", name, profileName, err)
	}
	waitLiveFSReady(t, bin, profileName, selected, 10*time.Minute)
	return selected
}

func waitLiveFSReady(t *testing.T, bin, profileName string, profile *config.Profile, timeout time.Duration) {
	t.Helper()
	client := liveFSClient(t, profile, authz.FSVolumeRead)
	probeLocalPath := filepath.Join(t.TempDir(), "ready.txt")
	if err := os.WriteFile(probeLocalPath, []byte("tdc fs live readiness probe\n"), 0o600); err != nil {
		t.Fatalf("write tdc fs readiness probe: %v", err)
	}
	probeRemotePath := fmt.Sprintf("/tdc-e2e-readiness-%d-%d.txt", os.Getpid(), time.Now().UnixNano())
	defer func() {
		cleanup := runLiveFSSetupCommand(t, bin, "--profile", profileName, "fs", "delete-file", "--file-system-name", profile.FSResourceName, "--path", probeRemotePath)
		if cleanup.exitCode != 0 && !isLiveFSNotFound(cleanup.stderr) {
			t.Logf("cleanup tdc fs readiness probe failed: exit=%d stderr=%s", cleanup.exitCode, strings.TrimSpace(cleanup.stderr))
		}
	}()
	deadline := time.Now().Add(timeout)
	var lastStatus apifs.StatusResponse
	var lastErr error
	var lastProbe commandResult
	consecutiveWriteProbes := 0
	for {
		status, err := client.Status(context.Background())
		if err == nil {
			lastStatus = status
			state := strings.ToLower(strings.TrimSpace(status.Status))
			if state == "" || (!strings.Contains(state, "provision") && !strings.Contains(state, "delet")) {
				lastProbe = runTDC(t, bin, "--profile", profileName, "fs", "copy-file", "--file-system-name", profile.FSResourceName, "--from-local", probeLocalPath, "--to-remote", probeRemotePath, "--overwrite")
				if lastProbe.exitCode == 0 {
					cleanup := runLiveFSSetupCommand(t, bin, "--profile", profileName, "fs", "delete-file", "--file-system-name", profile.FSResourceName, "--path", probeRemotePath)
					if cleanup.exitCode != 0 && !isLiveFSNotFound(cleanup.stderr) {
						cleanup.fail("delete tdc fs readiness probe")
					}
					consecutiveWriteProbes++
					if consecutiveWriteProbes >= 5 {
						return
					}
				} else {
					consecutiveWriteProbes = 0
					if !isLiveFSReadinessError(lastProbe.stderr) {
						lastProbe.fail("probe tdc fs data-plane readiness")
					}
				}
			}
		} else {
			lastErr = err
			if !isLiveFSReadinessError(err.Error()) {
				t.Fatalf("check tdc fs readiness for profile %q failed: %v", profile.Name, err)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for tdc fs resource %q in profile %q to become data-plane ready; last_status=%#v last_error=%v last_probe_stderr=%q", profile.FSResourceName, profile.Name, lastStatus, lastErr, strings.TrimSpace(lastProbe.stderr))
		}
		time.Sleep(5 * time.Second)
	}
}

func isLiveFSReadinessError(stderr string) bool {
	message := strings.ToLower(stderr)
	return strings.Contains(message, "storage backend unavailable") ||
		strings.Contains(message, "http 503") ||
		strings.Contains(message, "provision") ||
		strings.Contains(message, "service unavailable") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "no such host") ||
		strings.Contains(message, "network connectivity") ||
		strings.Contains(message, ": eof") ||
		strings.Contains(message, "timeout") ||
		strings.Contains(message, "unexpected eof")
}

func runLiveFSSetupCommand(t *testing.T, bin string, args ...string) commandResult {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		result := runTDC(t, bin, args...)
		if result.exitCode == 0 || !isLiveFSReadinessError(result.stderr) || time.Now().After(deadline) {
			return result
		}
		time.Sleep(5 * time.Second)
	}
}

func isLiveFSNotFound(stderr string) bool {
	return strings.Contains(strings.ToLower(stderr), "not found")
}

func TestIsLiveFSReadinessError(t *testing.T) {
	t.Parallel()
	for _, message := range []string{
		"tdc [ERROR]: fs ls: storage backend unavailable; contact support",
		"tdc [ERROR]: fs cp: HTTP 503:",
		"resource is still provisioning",
		"503 Service Unavailable",
		"connection reset by peer",
		"dial tcp: lookup drive9.ai: no such host",
		"API request failed: check network connectivity and try again",
		"status API request failed: EOF",
		"i/o timeout",
	} {
		if !isLiveFSReadinessError(message) {
			t.Fatalf("expected readiness error for %q", message)
		}
	}
	if isLiveFSReadinessError("tdc [ERROR]: authentication required") {
		t.Fatal("authentication errors must fail readiness immediately")
	}
}

func cleanupAutoCreatedLiveFSResource() {
	if os.Getenv("TDC_LIVE") != "1" {
		return
	}
	bin := os.Getenv("TDC_E2E_BIN")
	if bin == "" {
		_, _ = fmt.Fprintln(os.Stderr, "tdc live e2e cleanup warning: TDC_E2E_BIN is not set; cannot delete auto-created tdc fs resource")
		return
	}
	profileName := liveProfileNameFromEnv()
	name := liveFileSystemNameFromEnv()
	cmd := exec.Command(
		bin,
		"--profile", profileName,
		"fs", "delete-file-system",
		"--file-system-name", name,
		"--confirm-file-system-name", name,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "tdc live e2e cleanup warning: delete tdc fs resource %q failed: %v\n%s", name, err, string(output))
	}
}

func releaseAutoCreatedLiveFSResource(t *testing.T, bin, profileName string) {
	t.Helper()
	liveFSResourceMu.Lock()
	defer liveFSResourceMu.Unlock()
	if !liveFSResourceAutoCreated {
		return
	}
	name := liveFileSystemName(t)
	result := runTDC(
		t,
		bin,
		"--profile", profileName,
		"fs", "delete-file-system",
		"--file-system-name", name,
		"--confirm-file-system-name", name,
	)
	result.wantExitCode(0)
	liveFSResourceAutoCreated = false
}

func liveProfileName(t *testing.T) string {
	t.Helper()
	profileName := liveProfileNameFromEnv()
	if profileName != defaultLiveProfile {
		t.Fatalf("live e2e must use profile %q, got %q", defaultLiveProfile, profileName)
	}
	return profileName
}

func liveProfileNameFromEnv() string {
	profileName := os.Getenv("TDC_PROFILE")
	if profileName == "" {
		profileName = defaultLiveProfile
	}
	return profileName
}

func liveFileSystemName(t *testing.T) string {
	t.Helper()
	return liveFileSystemNameFromEnv()
}

func liveFileSystemNameFromEnv() string {
	name := strings.TrimSpace(os.Getenv("TDC_LIVE_FS_NAME"))
	if name == "" {
		name = "workspace"
	}
	return name
}

func liveProfile(t *testing.T) *config.Profile {
	t.Helper()
	liveProfileConfigureMu.Lock()
	defer liveProfileConfigureMu.Unlock()
	profileName := liveProfileName(t)
	load := func() (*config.Profile, error) {
		return auth.LoadProfile(context.Background(), config.LoadOptions{
			Profile:         profileName,
			ProfileExplicit: true,
		})
	}
	profile, err := load()
	if err != nil {
		t.Fatalf("load live e2e profile %q: %v\nconfigure it with: bin/tdc configure --profile %s", profileName, err, profileName)
	}
	if profile.ProjectID != "" {
		return profile
	}

	configured := runTDCWithInput(t, tdcBinary(t), "", []string{
		"TDC_REGION_CODE=" + profile.PlacementRegionCode,
		"TDC_PUBLIC_KEY=" + profile.TDCPublicKey,
		"TDC_PRIVATE_KEY=" + profile.TDCPrivateKey,
	}, "configure", "--profile", profileName, "--non-interactive")
	configured.wantExitCode(0)
	configured.wantStdoutContains(`"project_type": "tidbx_virtual"`)
	profile, err = load()
	if err != nil {
		t.Fatalf("reload live e2e profile %q after configure: %v", profileName, err)
	}
	if profile.ProjectID == "" {
		t.Fatalf("live e2e profile %q has no project_id after configure", profileName)
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
	provider := profile.FSCloudProvider
	regionCode := profile.FSRegionCode
	if provider == "" {
		provider = profile.CloudProvider
	}
	if regionCode == "" {
		regionCode = profile.RegionCode
	}
	endpoint, err := endpoints.NewResolver().ResolveFS(provider, regionCode)
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
