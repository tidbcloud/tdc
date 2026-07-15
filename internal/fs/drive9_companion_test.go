package fs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tidbcloud/tdc/internal/api/endpoints"
	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/config"
	"github.com/tidbcloud/tdc/internal/config/store"
	"github.com/tidbcloud/tdc/internal/dryrun"
)

type fakeDrive9Call struct {
	Args []string          `json:"args"`
	Env  map[string]string `json:"env"`
}

func TestDrive9CreateFileSystemStoresFlatCredentialsAndUsesCanonicalRegion(t *testing.T) {
	home := t.TempDir()
	companion, recordPath := buildFakeDrive9(t)
	t.Setenv("TDC_FAKE_DRIVE9_RECORD", recordPath)
	t.Setenv("DRIVE9_API_KEY", "ambient-drive9-key")
	profile := testProfile()

	result, err := testCompanionService(home, companion).CreateFileSystem(context.Background(), CreateFileSystemOptions{
		Profile:        profile,
		FileSystemName: "workspace",
	})
	if err != nil {
		t.Fatalf("CreateFileSystem failed: %v", err)
	}
	if result.FileSystemName != "workspace" || result.TenantID != "tenant-1" || result.RegionCode != "aws-us-east-1" || !result.CredentialsStored {
		t.Fatalf("unexpected result: %#v", result)
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

	createCall := requireFakeDrive9Call(t, recordPath, "create")
	wantArgs := []string{"create", "--json", "--name", "workspace", "--region-code", "aws-us-east-1"}
	if fmt.Sprint(createCall.Args) != fmt.Sprint(wantArgs) {
		t.Fatalf("create args = %#v, want %#v", createCall.Args, wantArgs)
	}
	if createCall.Env["DRIVE9_REGION_CODE"] != "aws-us-east-1" || createCall.Env["DRIVE9_SERVER"] != "https://fs.test" {
		t.Fatalf("unexpected region/server env: %#v", createCall.Env)
	}
	if createCall.Env["DRIVE9_PUBLIC_KEY"] != "public" || createCall.Env["DRIVE9_PRIVATE_KEY"] != "private" {
		t.Fatalf("missing TiDB Cloud keys in create env: %#v", createCall.Env)
	}
	if _, ok := createCall.Env["DRIVE9_API_KEY"]; ok {
		t.Fatalf("create should not pass an fs api key, env=%#v", createCall.Env)
	}
}

func TestDrive9CreateFileSystemFromEnvironmentProfileStoresDefaultProfile(t *testing.T) {
	home := t.TempDir()
	companion, recordPath := buildFakeDrive9(t)
	t.Setenv("TDC_FAKE_DRIVE9_RECORD", recordPath)
	profile := &config.Profile{
		Name:                config.DefaultProfile,
		Source:              "env",
		PlacementRegionCode: "aws-us-east-1",
		CloudProvider:       "aws",
		RegionCode:          "us-east-1",
		TDCPublicKey:        "env-public",
		TDCPrivateKey:       "env-private",
	}

	if _, err := testCompanionService(home, companion).CreateFileSystem(context.Background(), CreateFileSystemOptions{
		Profile:        profile,
		FileSystemName: "workspace",
	}); err != nil {
		t.Fatalf("CreateFileSystem failed: %v", err)
	}

	configDoc, err := store.ReadConfig(home)
	if err != nil {
		t.Fatalf("ReadConfig failed: %v", err)
	}
	if got := configDoc[config.DefaultProfile]; got.FSResourceName != "workspace" || got.FSTenantID != "tenant-1" {
		t.Fatalf("expected fs config under default profile, got %#v", got)
	}
	if _, ok := configDoc["env"]; ok {
		t.Fatalf("did not expect generated [env] config section: %#v", configDoc["env"])
	}
	credentialsDoc, err := store.ReadCredentials(home)
	if err != nil {
		t.Fatalf("ReadCredentials failed: %v", err)
	}
	if got := credentialsDoc[config.DefaultProfile]; got.FSAPIKey != "fs-secret" {
		t.Fatalf("expected fs api key under default profile, got %#v", got)
	}
	if _, ok := credentialsDoc["env"]; ok {
		t.Fatalf("did not expect generated [env] credentials section: %#v", credentialsDoc["env"])
	}
}

func TestDrive9DeleteFileSystemClearsFlatCredentials(t *testing.T) {
	home := t.TempDir()
	companion, recordPath := buildFakeDrive9(t)
	t.Setenv("TDC_FAKE_DRIVE9_RECORD", recordPath)
	profile := dataProfile()
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

	result, err := testCompanionService(home, companion).DeleteFileSystem(context.Background(), DeleteFileSystemOptions{
		Profile:               profile,
		FileSystemName:        "workspace",
		ConfirmFileSystemName: "workspace",
	})
	if err != nil {
		t.Fatalf("DeleteFileSystem failed: %v", err)
	}
	if !result.CredentialsRemoved || result.Status != "deleted" {
		t.Fatalf("unexpected delete result: %#v", result)
	}
	deleteCall := requireFakeDrive9Call(t, recordPath, "delete")
	if fmt.Sprint(deleteCall.Args) != fmt.Sprint([]string{"delete", "--json", "--yes"}) {
		t.Fatalf("delete args = %#v", deleteCall.Args)
	}
	if deleteCall.Env["DRIVE9_API_KEY"] != "fs-secret" || deleteCall.Env["DRIVE9_PUBLIC_KEY"] != "public" || deleteCall.Env["DRIVE9_PRIVATE_KEY"] != "private" {
		t.Fatalf("missing delete env: %#v", deleteCall.Env)
	}

	configDoc, err := store.ReadConfig(home)
	if err != nil {
		t.Fatalf("ReadConfig failed: %v", err)
	}
	if got := configDoc["stage"]; got.FSResourceName != "" || got.FSTenantID != "" || got.FSRegionCode != "" {
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

func TestDrive9CheckFileSystemDoesNotStatRemoteWithoutFSAPIKey(t *testing.T) {
	companion, recordPath := buildFakeDrive9(t)
	t.Setenv("TDC_FAKE_DRIVE9_RECORD", recordPath)

	result, err := testCompanionService(t.TempDir(), companion).CheckFileSystem(context.Background(), CheckFileSystemOptions{Profile: testProfile()})
	if err != nil {
		t.Fatalf("CheckFileSystem failed: %v", err)
	}
	if result.Status != "warning" {
		t.Fatalf("expected warning check, got %#v", result)
	}
	if !hasCheck(result.Checks, "remote_status", "warning") {
		t.Fatalf("expected warning remote status check: %#v", result.Checks)
	}
	for _, call := range readFakeDrive9Calls(t, recordPath) {
		if len(call.Args) >= 2 && call.Args[0] == "fs" && call.Args[1] == "stat" {
			t.Fatalf("remote stat should not run without fs_api_key: %#v", call.Args)
		}
	}
}

func TestDrive9DataPlaneCommandsTranslateToCompanion(t *testing.T) {
	home := t.TempDir()
	companion, recordPath := buildFakeDrive9(t)
	t.Setenv("TDC_FAKE_DRIVE9_RECORD", recordPath)
	localFile := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(localFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := testCompanionService(home, companion)

	result, err := service.CopyFile(context.Background(), CopyFileOptions{
		Profile:   dataProfile(),
		FromLocal: localFile,
		ToRemote:  "/workspace/README.md",
		Overwrite: true,
		Tags:      map[string]string{"owner": "agent"},
	})
	if err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}
	if result.SourcePath != localFile || result.TargetPath != "/workspace/README.md" || result.Status != "copied" {
		t.Fatalf("unexpected copy result: %#v", result)
	}
	data, err := service.ReadFile(context.Background(), ReadFileOptions{Profile: dataProfile(), Path: "/workspace/README.md"})
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "file bytes" {
		t.Fatalf("read data = %q", data)
	}

	cpCall := requireFakeDrive9Call(t, recordPath, "fs", "cp")
	if !containsArg(cpCall.Args, drive9RemoteMust("/workspace/README.md")) || !containsArg(cpCall.Args, "--tag") {
		t.Fatalf("copy args missing remote path/tag: %#v", cpCall.Args)
	}
	if cpCall.Env["DRIVE9_API_KEY"] != "fs-secret" {
		t.Fatalf("copy did not receive fs api key: %#v", cpCall.Env)
	}
	if _, ok := cpCall.Env["DRIVE9_PUBLIC_KEY"]; ok {
		t.Fatalf("data-plane copy should not pass TiDB Cloud public key: %#v", cpCall.Env)
	}
	catCall := requireFakeDrive9Call(t, recordPath, "fs", "cat")
	if fmt.Sprint(catCall.Args) != fmt.Sprint([]string{"fs", "cat", drive9RemoteMust("/workspace/README.md")}) {
		t.Fatalf("cat args = %#v", catCall.Args)
	}
}

func TestDrive9MissingCompanionIsActionable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := Service{CompanionPath: filepath.Join(t.TempDir(), "missing-tdc-drive9")}.ReadFile(context.Background(), ReadFileOptions{
		Profile: dataProfile(),
		Path:    "/workspace/README.md",
	})
	if err == nil {
		t.Fatal("expected missing companion error")
	}
	if message := apperr.MessageFor(err); !strings.Contains(message, "tdc fs requires the Drive9 companion binary") {
		t.Fatalf("unexpected error: %q", message)
	}
}

func TestDryRunCreateFileSystemUsesRedactedProvisionShape(t *testing.T) {
	profile := testProfile()
	result, err := Service{Resolver: supportedFSManifestResolver("https://fs.test")}.DryRunCreateFileSystem(context.Background(), "tdc fs create-file-system", CreateFileSystemOptions{
		Profile:        profile,
		FileSystemName: "workspace",
	})
	if err != nil {
		t.Fatalf("DryRunCreateFileSystem failed: %v", err)
	}
	if result.Operation != "create_file_system" || result.Request.Path != "/v1/provision" {
		t.Fatalf("unexpected dry-run result: %#v", result)
	}
	bodyBytes, err := json.Marshal(result.Request.Body)
	if err != nil {
		t.Fatalf("marshal dry-run body: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("decode dry-run body: %v", err)
	}
	if body["public_key"] != "[configured]" || body["private_key"] != "[redacted]" {
		t.Fatalf("dry-run leaked credentials: %#v", body)
	}
	if _, ok := body["tidbcloud_spending_limit"]; ok {
		t.Fatalf("dry-run should not include spending limit: %#v", body)
	}
	if !hasDryRunCheck(result.Checks, "endpoint_selection", "passed") {
		t.Fatalf("expected endpoint dry-run check: %#v", result.Checks)
	}
}

func TestDrive9VaultHelpers(t *testing.T) {
	scopes, err := drive9VaultGrantScopes([]string{"db-prod", "db-prod/DB_URL", "/n/vault/canonical/TOKEN"})
	if err != nil {
		t.Fatalf("drive9VaultGrantScopes failed: %v", err)
	}
	want := []string{"db-prod", "db-prod/DB_URL", "canonical/TOKEN"}
	if fmt.Sprint(scopes) != fmt.Sprint(want) {
		t.Fatalf("scopes = %#v, want %#v", scopes, want)
	}
	if !isTransientDrive9Error(fmt.Errorf(`vault rm: Delete "https://example/v1/vault/secrets/x": EOF`)) {
		t.Fatal("EOF should be treated as transient")
	}
	if isTransientDrive9Error(fmt.Errorf("vault rm: not found")) {
		t.Fatal("not found should not be treated as transient")
	}
}

func buildFakeDrive9(t *testing.T) (binPath, recordPath string) {
	t.Helper()
	dir := t.TempDir()
	recordPath = filepath.Join(dir, "calls.jsonl")
	sourcePath := filepath.Join(dir, "fake_drive9.go")
	source := `package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type callRecord struct {
	Args []string          ` + "`json:\"args\"`" + `
	Env  map[string]string ` + "`json:\"env\"`" + `
}

func main() {
	args := os.Args[1:]
	record(args)
	if len(args) > 0 && args[0] == "--help" {
		fmt.Println("fake drive9 help")
		return
	}
	if len(args) == 0 {
		return
	}
	switch {
	case args[0] == "create":
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{
			"tenant_id":      "tenant-1",
			"api_key":        "fs-secret",
			"status":         "active",
			"cloud_provider": "aws",
			"region_code":    os.Getenv("DRIVE9_REGION_CODE"),
			"server":         os.Getenv("DRIVE9_SERVER"),
		})
	case args[0] == "delete":
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"status": "deleted"})
	case len(args) >= 2 && args[0] == "fs" && args[1] == "cat":
		fmt.Fprint(os.Stdout, "file bytes")
	case len(args) >= 2 && args[0] == "fs" && args[1] == "stat":
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"path": ":/", "size": 12, "isdir": false, "revision": 3})
	default:
		return
	}
}

func record(args []string) {
	path := os.Getenv("TDC_FAKE_DRIVE9_RECORD")
	if path == "" {
		return
	}
	env := map[string]string{}
	for _, key := range []string{"DRIVE9_API_KEY", "DRIVE9_SERVER", "DRIVE9_REGION_CODE", "DRIVE9_PUBLIC_KEY", "DRIVE9_PRIVATE_KEY", "DRIVE9_VAULT_TOKEN", "HOME"} {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(callRecord{Args: args, Env: env})
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatalf("write fake companion source: %v", err)
	}
	binPath = filepath.Join(dir, "tdc-drive9")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", binPath, sourcePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fake companion: %v\n%s", err, output)
	}
	return binPath, recordPath
}

func readFakeDrive9Calls(t *testing.T, recordPath string) []fakeDrive9Call {
	t.Helper()
	file, err := os.Open(recordPath)
	if err != nil {
		t.Fatalf("open fake companion record: %v", err)
	}
	defer file.Close()
	var calls []fakeDrive9Call
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var call fakeDrive9Call
		if err := json.Unmarshal(scanner.Bytes(), &call); err != nil {
			t.Fatalf("decode fake companion record %q: %v", scanner.Text(), err)
		}
		calls = append(calls, call)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan fake companion record: %v", err)
	}
	return calls
}

func requireFakeDrive9Call(t *testing.T, recordPath string, prefix ...string) fakeDrive9Call {
	t.Helper()
	for _, call := range readFakeDrive9Calls(t, recordPath) {
		if hasArgPrefix(call.Args, prefix) {
			return call
		}
	}
	t.Fatalf("missing fake companion call with prefix %#v", prefix)
	return fakeDrive9Call{}
}

func hasArgPrefix(args, prefix []string) bool {
	if len(args) < len(prefix) {
		return false
	}
	for i := range prefix {
		if args[i] != prefix[i] {
			return false
		}
	}
	return true
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
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

func dataProfile() *config.Profile {
	profile := testProfile()
	profile.FSResourceName = "workspace"
	profile.FSTenantID = "tenant-1"
	profile.FSCloudProvider = "aws"
	profile.FSRegionCode = "us-east-1"
	profile.FSPlacementRegionCode = "aws-us-east-1"
	profile.FSAPIKey = "fs-secret"
	return profile
}

func testCompanionService(home, companion string) Service {
	return Service{
		HomeDir:       home,
		CompanionPath: companion,
		Resolver:      supportedFSManifestResolver("https://fs.test"),
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
