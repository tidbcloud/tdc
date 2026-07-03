package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Icemap/tdc/internal/api"
	"github.com/Icemap/tdc/internal/api/endpoints"
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
		{"organization", "help"},
		{"db", "create-db-cluster", "help"},
		{"db", "list-db-clusters", "help"},
		{"fs", "create-file-system", "help"},
		{"fs", "copy-file", "help"},
		{"fs", "read-file", "help"},
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

	placeholderCommands := [][]string{
		{"cli", "check-update"},
		{"cli", "update"},
		{"fs", "mount-file-system"},
		{"fs", "unmount-file-system"},
	}
	for _, args := range placeholderCommands {
		result := runTDC(t, bin, args...)
		result.wantExitCode(2)
		result.wantStderrContains("is not implemented yet")
	}
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
	if string(downloaded) != content {
		t.Fatalf("downloaded file mismatch: got %q want %q", downloaded, content)
	}

	deleteMoved := runTDC(t, bin, "--profile", profileName, "fs", "delete-file", "--path", movedPath)
	deleteMoved.wantExitCode(0)
	deleteMoved.wantStdoutContains(`"status": "deleted"`)

	deleteRoot := runTDC(t, bin, "--profile", profileName, "fs", "delete-file", "--path", rootPath, "--recursive")
	deleteRoot.wantExitCode(0)
	deleteRoot.wantStdoutContains(`"status": "deleted"`)
	deleted = true
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
