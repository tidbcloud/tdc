package fs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tidbcloud/tdc/internal/api/endpoints"
	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/config"
	"github.com/tidbcloud/tdc/internal/config/store"
	"github.com/tidbcloud/tdc/internal/dryrun"
)

func TestCreateFileSystemStoresFlatCredentialsAndRedactsOutput(t *testing.T) {
	home := t.TempDir()
	profile := testProfile()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/provision" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("provision Authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["public_key"] != "public" || body["private_key"] != "private" || body["tidbcloud_spending_limit"] != float64(0) {
			t.Fatalf("unexpected request body: %#v", body)
		}
		if _, ok := body["file_system_name"]; ok {
			t.Fatalf("unexpected request body: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tenant_id": "tenant-1",
			"api_key":   "fs-secret",
			"status":    "active",
		})
	}))
	defer server.Close()

	result, err := testService(home, server.URL).CreateFileSystem(context.Background(), CreateFileSystemOptions{
		Profile:        profile,
		FileSystemName: "workspace",
	})
	if err != nil {
		t.Fatalf("CreateFileSystem failed: %v", err)
	}
	if result.FileSystemName != "workspace" || result.TenantID != "tenant-1" || !result.CredentialsStored {
		t.Fatalf("unexpected result: %#v", result)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "fs-secret") {
		t.Fatalf("result leaked api key: %s", data)
	}

	configDoc, err := store.ReadConfig(home)
	if err != nil {
		t.Fatalf("ReadConfig failed: %v", err)
	}
	if got := configDoc["stage"]; got.FSResourceName != "workspace" || got.FSTenantID != "tenant-1" || got.FSCloudProvider != "aws" || got.FSRegionCode != "aws-us-east-1" {
		t.Fatalf("unexpected fs config: %#v", got)
	}
	credentialsDoc, err := store.ReadCredentials(home)
	if err != nil {
		t.Fatalf("ReadCredentials failed: %v", err)
	}
	if got := credentialsDoc["stage"]; got.FSAPIKey != "fs-secret" {
		t.Fatalf("fs api key not stored flat under profile: %#v", got)
	}
}

func TestDeleteFileSystemUsesBearerAndClearsFlatCredentials(t *testing.T) {
	home := t.TempDir()
	profile := testProfile()
	profile.FSResourceName = "workspace"
	profile.FSTenantID = "tenant-1"
	profile.FSPlacementRegionCode = "aws-us-east-1"
	profile.FSCloudProvider = "aws"
	profile.FSRegionCode = "us-east-1"
	profile.FSAPIKey = "fs-secret"
	if err := store.WriteProfile(home, profile.Name, store.ConfigProfile{
		RegionCode:      profile.PlacementRegionCode,
		FSResourceName:  profile.FSResourceName,
		FSTenantID:      profile.FSTenantID,
		FSCloudProvider: profile.FSCloudProvider,
		FSRegionCode:    profile.FSPlacementRegionCode,
	}, store.CredentialsProfile{
		TDCPublicKey:  profile.TDCPublicKey,
		TDCPrivateKey: profile.TDCPrivateKey,
		FSAPIKey:      profile.FSAPIKey,
	}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/tenant" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["public_key"] != "public" || body["private_key"] != "private" {
			t.Fatalf("unexpected request body: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tenant_id": "tenant-1",
			"status":    "deleting",
		})
	}))
	defer server.Close()

	result, err := testService(home, server.URL).DeleteFileSystem(context.Background(), DeleteFileSystemOptions{
		Profile:               profile,
		FileSystemName:        "workspace",
		ConfirmFileSystemName: "workspace",
	})
	if err != nil {
		t.Fatalf("DeleteFileSystem failed: %v", err)
	}
	if !result.CredentialsRemoved || result.Status != "deleted" || result.RemoteDeletionState != "deleting" {
		t.Fatalf("unexpected delete result: %#v", result)
	}

	configDoc, err := store.ReadConfig(home)
	if err != nil {
		t.Fatalf("ReadConfig failed: %v", err)
	}
	if got := configDoc["stage"]; got.FSResourceName != "" || got.FSTenantID != "" || got.CloudProvider != "" || got.RegionCode != "aws-us-east-1" {
		t.Fatalf("unexpected config after delete: %#v", got)
	}
	credentialsDoc, err := store.ReadCredentials(home)
	if err != nil {
		t.Fatalf("ReadCredentials failed: %v", err)
	}
	if got := credentialsDoc["stage"]; got.FSAPIKey != "" || got.TDCPublicKey != "public" {
		t.Fatalf("unexpected credentials after delete: %#v", got)
	}
}

func TestCheckFileSystemReportsMissingEndpointAsStructuredFailure(t *testing.T) {
	result, err := Service{Resolver: unsupportedFSManifestResolver()}.CheckFileSystem(context.Background(), CheckFileSystemOptions{Profile: testProfile()})
	if err != nil {
		t.Fatalf("CheckFileSystem failed: %v", err)
	}
	if result.Status != "failed" {
		t.Fatalf("expected failed check, got %#v", result)
	}
	if !hasCheck(result.Checks, "endpoint_selection", "failed") {
		t.Fatalf("expected failed endpoint check: %#v", result.Checks)
	}
}

func TestCheckFileSystemSkipsRemoteWithoutFSAPIKey(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("remote status should not be called without fs_api_key")
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).CheckFileSystem(context.Background(), CheckFileSystemOptions{Profile: testProfile()})
	if err != nil {
		t.Fatalf("CheckFileSystem failed: %v", err)
	}
	if called {
		t.Fatal("remote status was called")
	}
	if result.Status != "warning" || result.Remote != nil {
		t.Fatalf("unexpected check result: %#v", result)
	}
	if !hasCheck(result.Checks, "remote_status", "warning") {
		t.Fatalf("expected warning remote status check: %#v", result.Checks)
	}
}

func TestCheckFileSystemUsesBearerWhenCredentialsExist(t *testing.T) {
	profile := testProfile()
	profile.FSResourceName = "workspace"
	profile.FSTenantID = "tenant-1"
	profile.FSCloudProvider = "aws"
	profile.FSRegionCode = "us-east-1"
	profile.FSAPIKey = "fs-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/status" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":    "ok",
			"tenant_id": "tenant-1",
		})
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).CheckFileSystem(context.Background(), CheckFileSystemOptions{Profile: profile})
	if err != nil {
		t.Fatalf("CheckFileSystem failed: %v", err)
	}
	if result.Status != "passed" || result.Remote == nil || result.Remote.Status != "ok" {
		t.Fatalf("unexpected check result: %#v", result)
	}
	if !hasCheck(result.Checks, "remote_status", "passed") {
		t.Fatalf("expected passed remote status check: %#v", result.Checks)
	}
}

func TestDryRunCreateFileSystemRedactsNativeCredentials(t *testing.T) {
	profile := testProfile()
	profile.TDCPublicKey = "pk-secret"
	profile.TDCPrivateKey = "sk-secret"
	result, err := Service{Resolver: supportedFSManifestResolver("https://fs.test")}.DryRunCreateFileSystem(context.Background(), "tdc fs create-file-system", CreateFileSystemOptions{
		Profile:        profile,
		FileSystemName: "workspace",
	})
	if err != nil {
		t.Fatalf("DryRunCreateFileSystem failed: %v", err)
	}
	if !result.DryRun || result.Request.Path != "/v1/provision" {
		t.Fatalf("unexpected dry-run: %#v", result)
	}
	if !hasDryRunCheck(result.Checks, "endpoint_selection", "passed") {
		t.Fatalf("expected passed endpoint check: %#v", result.Checks)
	}
	raw, err := json.Marshal(result.Request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "pk-secret") || strings.Contains(string(raw), "sk-secret") {
		t.Fatalf("dry-run leaked credentials: %s", raw)
	}
}

func TestCreateFileSystemRequiresEndpointForRealMutation(t *testing.T) {
	_, err := Service{Resolver: unsupportedFSManifestResolver()}.CreateFileSystem(context.Background(), CreateFileSystemOptions{
		Profile:        testProfile(),
		FileSystemName: "workspace",
	})
	if err == nil {
		t.Fatal("expected missing endpoint to fail")
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "tdc fs is not available") {
		t.Fatalf("unexpected error: %q", got)
	}
}

func testProfile() *config.Profile {
	return &config.Profile{
		Name:                "stage",
		PlacementRegionCode: "aws-us-east-1",
		CloudProvider:       "aws",
		RegionCode:          "us-east-1",
		TDCPublicKey:        "public",
		TDCPrivateKey:       "private",
	}
}

func testService(home, baseURL string) Service {
	return Service{
		HomeDir: home,
		Resolver: endpoints.Resolver{
			FSBaseURLs: map[endpoints.ProviderRegion]string{
				{Provider: "aws", Region: "us-east-1"}: baseURL,
			},
		},
	}
}

func supportedFSManifestResolver(baseURL string) endpoints.Resolver {
	return endpoints.Resolver{
		FSManifest: &endpoints.FSRegionManifest{
			Regions: []endpoints.FSRegionManifestEntry{
				{
					RegionCode:    "aws-us-east-1",
					Mode:          endpoints.DefaultFSMode,
					ServerURL:     baseURL,
					CloudProvider: "aws",
					TiDBRegion:    "us-east-1",
				},
			},
		},
	}
}

func unsupportedFSManifestResolver() endpoints.Resolver {
	return endpoints.Resolver{
		FSManifest: &endpoints.FSRegionManifest{
			Regions: []endpoints.FSRegionManifestEntry{
				{
					RegionCode:    "aws-ap-southeast-1",
					Mode:          endpoints.DefaultFSMode,
					ServerURL:     "https://fs.aws-sg.test",
					CloudProvider: "aws",
					TiDBRegion:    "ap-southeast-1",
				},
			},
		},
	}
}

func hasCheck(checks []Check, name, status string) bool {
	for _, check := range checks {
		if check.Name == name && check.Status == status {
			return true
		}
	}
	return false
}

func hasDryRunCheck(checks []dryrun.Check, name, status string) bool {
	for _, check := range checks {
		if check.Name == name && check.Status == status {
			return true
		}
	}
	return false
}
