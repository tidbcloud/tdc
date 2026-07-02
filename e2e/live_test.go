package e2e

import (
	"context"
	"encoding/json"
	"os"
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
	}
	for _, args := range helpCommands {
		result := runTDC(t, bin, args...)
		result.wantExitCode(0)
		result.wantStdoutContains("Usage:")
	}

	mutatingDryRunCommands := [][]string{
		{"db", "create-db-cluster-branch"},
		{"db", "delete-db-cluster-branch"},
		{"db", "prepare-db-query-access"},
		{"fs", "create-file-system"},
		{"fs", "delete-file-system"},
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

	readOnlyCommands := [][]string{
		{"organization", "list-projects"},
		{"db", "list-db-clusters"},
		{"db", "describe-db-cluster"},
		{"db", "list-db-cluster-branches"},
		{"db", "describe-db-cluster-branch"},
		{"db", "create-db-connection-string"},
		{"db", "execute-sql-statement"},
		{"fs", "check-file-system"},
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
		{"db", "create-db-cluster-branch"},
		{"db", "list-db-cluster-branches"},
		{"db", "describe-db-cluster-branch"},
		{"db", "delete-db-cluster-branch"},
		{"db", "prepare-db-query-access"},
		{"db", "create-db-connection-string"},
		{"db", "execute-sql-statement"},
		{"fs", "create-file-system"},
		{"fs", "delete-file-system"},
		{"fs", "check-file-system"},
		{"fs", "copy-file"},
		{"fs", "read-file"},
		{"fs", "list-files"},
		{"fs", "describe-file"},
		{"fs", "move-file"},
		{"fs", "delete-file"},
		{"fs", "create-directory"},
		{"fs", "search-file-content"},
		{"fs", "find-files"},
		{"fs", "mount-file-system"},
		{"fs", "unmount-file-system"},
	}
	for _, args := range placeholderCommands {
		result := runTDC(t, bin, args...)
		result.wantExitCode(2)
		result.wantStderrContains("is not implemented yet")
	}
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

	described := waitLiveCluster(t, bin, profileName, clusterID, func(cluster liveCluster) bool {
		return cluster.ID == clusterID && cluster.DisplayName == clusterName && cluster.State == "ACTIVE"
	}, 12*time.Minute, "become ACTIVE after create")
	if described.ClusterPlan != "" && described.ClusterPlan != "STARTER" {
		t.Fatalf("expected STARTER cluster, got %#v", described)
	}

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

func decodeLiveCluster(t *testing.T, result commandResult) liveCluster {
	t.Helper()
	var cluster liveCluster
	if err := json.Unmarshal([]byte(result.stdout), &cluster); err != nil {
		t.Fatalf("decode cluster output: %v\n%s", err, result.stdout)
	}
	return cluster
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
