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
		{"db", "create-db-cluster"},
		{"db", "update-db-cluster"},
		{"db", "delete-db-cluster"},
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

	placeholderCommands := [][]string{
		{"cli", "check-update"},
		{"cli", "update"},
		{"db", "create-db-cluster"},
		{"db", "list-db-clusters"},
		{"db", "describe-db-cluster"},
		{"db", "update-db-cluster"},
		{"db", "delete-db-cluster"},
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

func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv("TDC_LIVE") != "1" {
		t.Skip("TDC_LIVE=1 is required; run make live-e2e")
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
