package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/authz"
	"github.com/tidbcloud/tdc/internal/config/store"
	"github.com/tidbcloud/tdc/internal/dryrun"
	"github.com/tidbcloud/tdc/internal/oplog"
	"github.com/tidbcloud/tdc/internal/version"
)

func TestHelpCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "root help",
			args: []string{"help"},
			want: "Commands:",
		},
		{
			name: "db help",
			args: []string{"db", "help"},
			want: "create-db-cluster",
		},
		{
			name: "fs help",
			args: []string{"fs", "help"},
			want: "mount-file-system",
		},
		{
			name: "subcommand help",
			args: []string{"fs", "mount-file-system", "help"},
			want: "Mount a tdc fs resource locally.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _, err := executeForTest(tt.args...)
			if err != nil {
				t.Fatalf("expected help to succeed, got %v", err)
			}
			if !strings.Contains(stdout, tt.want) {
				t.Fatalf("expected output to contain %q, got:\n%s", tt.want, stdout)
			}
		})
	}
}

func TestVersionWorksAtEveryLevel(t *testing.T) {
	tests := [][]string{
		{"--version"},
		{"db", "--version"},
		{"fs", "mount-file-system", "--version"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, _, err := executeForTest(args...)
			if err != nil {
				t.Fatalf("expected version to succeed, got %v", err)
			}
			if got := strings.TrimSpace(stdout); got != testVersion().String() {
				t.Fatalf("expected %q, got %q", testVersion().String(), got)
			}
		})
	}
}

func TestUnknownCommandReturnsActionableError(t *testing.T) {
	_, _, err := executeForTest("db", "missing-command")
	if err == nil {
		t.Fatal("expected unknown command to fail")
	}
	if apperr.ExitCodeFor(err) != 2 {
		t.Fatalf("expected exit code 2, got %d", apperr.ExitCodeFor(err))
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "unknown command") {
		t.Fatalf("expected unknown command message, got %q", got)
	}
}

func TestContextCanceledIsRenderedAsInterrupted(t *testing.T) {
	err := normalizeError(context.Canceled)
	if err == nil {
		t.Fatal("expected interrupted error")
	}
	if got := apperr.ExitCodeFor(err); got != 130 {
		t.Fatalf("expected exit code 130, got %d", got)
	}
	if got := apperr.MessageFor(err); got != "interrupted" {
		t.Fatalf("expected interrupted message, got %q", got)
	}
}

func TestShortHelpFlagIsRejected(t *testing.T) {
	stdout, _, err := executeForTest("-h")
	if err == nil {
		t.Fatal("expected -h to fail")
	}
	if stdout != "" {
		t.Fatalf("expected no stdout, got %q", stdout)
	}
	if apperr.ExitCodeFor(err) != 2 {
		t.Fatalf("expected exit code 2, got %d", apperr.ExitCodeFor(err))
	}
}

func TestCommandOperationLogRecordsSafeSummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TDC_LOGGING", "on")

	_, _, err := executeForTest(
		"configure",
		"--profile", "ci",
		"--region-code", "aws-us-east-1",
		"--tdc-public-key", "public-secret",
		"--tdc-private-key", "private-secret",
		"--non-interactive",
	)
	if err != nil {
		t.Fatalf("configure failed: %v", err)
	}

	data, err := os.ReadFile(store.LogPath(home))
	if err != nil {
		t.Fatalf("read operation log: %v", err)
	}
	if strings.Contains(string(data), "public-secret") || strings.Contains(string(data), "private-secret") {
		t.Fatalf("operation log leaked secret values:\n%s", string(data))
	}
	var event oplog.Event
	if err := json.Unmarshal(bytes.TrimSpace(data), &event); err != nil {
		t.Fatalf("decode operation log: %v\n%s", err, string(data))
	}
	if event.Type != "command" || event.Command != "tdc configure" || event.Profile != "ci" || event.RegionCode != "aws-us-east-1" {
		t.Fatalf("unexpected command event: %#v", event)
	}
	if !containsString(event.FlagNames, "tdc-private-key") || !containsString(event.FlagNames, "tdc-public-key") {
		t.Fatalf("expected flag names only, got %#v", event.FlagNames)
	}
}

func TestCommandOperationLogCanBeDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TDC_LOGGING", "off")

	_, _, err := executeForTest("help")
	if err != nil {
		t.Fatalf("help failed: %v", err)
	}
	if _, err := os.Stat(store.LogPath(home)); !os.IsNotExist(err) {
		t.Fatalf("expected no operation log file, got err=%v", err)
	}
}

func TestHelpOutputDoesNotExposeShortFlags(t *testing.T) {
	stdout, _, err := executeForTest("--help")
	if err != nil {
		t.Fatalf("expected --help to succeed, got %v", err)
	}
	if strings.Contains(stdout, "-h,") || strings.Contains(stdout, "-v,") {
		t.Fatalf("help output exposes a short flag:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--region string") {
		t.Fatalf("help output does not include global --region flag:\n%s", stdout)
	}
}

func TestNoCommandDefinesShortFlags(t *testing.T) {
	root := NewRootCommand(testVersion())
	visitCommands(root, func(cmd *cobra.Command) {
		cmd.InitDefaultHelpFlag()
		cmd.InitDefaultVersionFlag()
	})
	if HasShorthand(root) {
		t.Fatal("expected command tree to avoid short flags")
	}
}

func TestServiceCommandsDeclarePermissions(t *testing.T) {
	root := NewRootCommand(testVersion())
	visitCommands(root, func(cmd *cobra.Command) {
		if cmd == root || cmd.Name() == "help" || cmd.HasSubCommands() {
			return
		}
		path := cmd.CommandPath()
		if !strings.HasPrefix(path, "tdc db ") &&
			!strings.HasPrefix(path, "tdc fs ") &&
			!strings.HasPrefix(path, "tdc fs-git ") &&
			!strings.HasPrefix(path, "tdc fs-journal ") &&
			!strings.HasPrefix(path, "tdc fs-vault ") &&
			!strings.HasPrefix(path, "tdc organization ") {
			return
		}
		if _, err := authz.ForCommand(path); err != nil {
			t.Fatalf("missing permission for %s: %v", path, err)
		}
	})
}

func TestFSUnixAliasesResolveToCanonicalCommands(t *testing.T) {
	root := NewRootCommand(testVersion())
	fs := findChildCommand(root, "fs")
	if fs == nil {
		t.Fatal("missing fs command")
	}

	tests := []struct {
		canonical string
		alias     string
	}{
		{canonical: "copy-file", alias: "cp"},
		{canonical: "read-file", alias: "cat"},
		{canonical: "list-files", alias: "ls"},
		{canonical: "describe-file", alias: "stat"},
		{canonical: "move-file", alias: "mv"},
		{canonical: "delete-file", alias: "rm"},
		{canonical: "create-directory", alias: "mkdir"},
		{canonical: "chmod-file", alias: "chmod"},
		{canonical: "create-symlink", alias: "symlink"},
		{canonical: "create-hardlink", alias: "hardlink"},
		{canonical: "search-file-content", alias: "grep"},
		{canonical: "find-files", alias: "find"},
		{canonical: "mount-file-system", alias: "mount"},
		{canonical: "drain-file-system", alias: "drain"},
		{canonical: "unmount-file-system", alias: "umount"},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			canonical := findChildCommand(fs, tt.canonical)
			if canonical == nil {
				t.Fatalf("missing canonical command %s", tt.canonical)
			}
			if !containsString(canonical.Aliases, tt.alias) {
				t.Fatalf("expected %s aliases to contain %q, got %#v", tt.canonical, tt.alias, canonical.Aliases)
			}

			resolved, _, err := root.Find([]string{"fs", tt.alias})
			if err != nil {
				t.Fatalf("resolve alias: %v", err)
			}
			if resolved.Name() != tt.canonical {
				t.Fatalf("expected alias %s to resolve to %s, got %s", tt.alias, tt.canonical, resolved.Name())
			}
			path := resolved.CommandPath()
			if want := "tdc fs " + tt.canonical; path != want {
				t.Fatalf("expected canonical path %q, got %q", want, path)
			}
			if _, err := authz.ForCommand(path); err != nil {
				t.Fatalf("alias %s resolved to command without permission: %v", tt.alias, err)
			}

			stdout, _, err := executeForTest("fs", tt.alias, "help")
			if err != nil {
				t.Fatalf("expected alias help to succeed, got %v", err)
			}
			if !strings.Contains(stdout, "Aliases:") || !strings.Contains(stdout, "  "+tt.alias) {
				t.Fatalf("expected alias help to list %q, got:\n%s", tt.alias, stdout)
			}
		})
	}
}

func TestFSAdjunctCommandsRequireConfiguredFSResource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TDC_REGION_CODE", "")
	t.Setenv("TDC_PUBLIC_KEY", "")
	t.Setenv("TDC_PRIVATE_KEY", "")
	writeCompleteProfile(t, home, "default")

	_, _, err := executeForTest("fs-vault", "list-secrets")
	if err == nil {
		t.Fatal("expected missing tdc fs resource to fail")
	}
	if got := apperr.ExitCodeFor(err); got != 3 {
		t.Fatalf("expected auth exit code 3, got %d", got)
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "tdc fs is not configured") || !strings.Contains(got, "tdc fs create-file-system") {
		t.Fatalf("unexpected message %q", got)
	}
}

func TestUpdateCheckUsesReleaseMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{
			"tag_name": "v0.1.0",
			"html_url": "https://github.com/tidbcloud/tdc/releases/tag/v0.1.0",
			"assets": [
				{
					"name": "tdc_linux_amd64.tar.gz",
					"browser_download_url": "https://github.com/tidbcloud/tdc/releases/download/v0.1.0/tdc_linux_amd64.tar.gz"
				}
			]
		}`))
	}))
	defer server.Close()
	t.Setenv("TDC_RELEASE_API_BASE_URL", server.URL)

	stdout, _, err := executeForTest("update", "--check", "--query", "latest_version")
	if err != nil {
		t.Fatalf("expected update --check to succeed, got %v", err)
	}
	if got := stdout; got != "\"0.1.0\"\n" {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestUpdateCheckRejectsApplyFlags(t *testing.T) {
	_, _, err := executeForTest("update", "--check", "--dry-run")
	if err == nil {
		t.Fatal("expected update --check --dry-run to fail")
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "--dry-run cannot be used with --check") {
		t.Fatalf("unexpected message: %q", got)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestCLIUpdateRefusesUnownedLocalBuild(t *testing.T) {
	_, _, err := executeForTest("update", "--dry-run")
	if err == nil {
		t.Fatal("expected local build update to fail")
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "not owned by tdc") {
		t.Fatalf("unexpected message: %q", got)
	}
}

func TestControlPlaneCommandSpecRendersImplementedResult(t *testing.T) {
	root := newCommand(commandSpec{
		Use: "tdc",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}, testVersion())
	root.PersistentFlags().String("output", "json", "output format")
	root.PersistentFlags().String("query", "", "JMESPath query applied to JSON output")
	root.AddCommand(newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "implemented-command",
		Short:      "Implemented command.",
		Mutation:   readOnlyCommand,
		Permission: authz.OrganizationProjectRead,
		Run: func(commandContext) (any, error) {
			return map[string]any{
				"items": []map[string]string{
					{"id": "item-1"},
				},
			}, nil
		},
	}, testVersion()))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), root, []string{"implemented-command", "--query", "items[0].id"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected implemented command to succeed, got %v", err)
	}
	if got := stdout.String(); got != "\"item-1\"\n" {
		t.Fatalf("unexpected output %q", got)
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestControlPlaneCommandSpecUsesCustomDryRun(t *testing.T) {
	root := newCommand(commandSpec{Use: "tdc"}, testVersion())
	root.PersistentFlags().String("output", "json", "output format")
	root.PersistentFlags().String("query", "", "JMESPath query applied to JSON output")
	root.AddCommand(newControlPlaneCommand(controlPlaneCommandSpec{
		Use:        "create-resource",
		Short:      "Create a resource.",
		Mutation:   mutatingCommand,
		Permission: authz.StarterClusterCreate,
		Run: func(ctx commandContext) (any, error) {
			return nil, apperr.NotImplemented(ctx.CommandPath())
		},
		DryRun: func(ctx commandContext) (dryrun.Result, error) {
			return dryrun.New(
				ctx.CommandPath(),
				"create_resource",
				dryrun.RequestSummary{
					Method: "POST",
					Path:   "/v1/resources",
					Body: map[string]string{
						"name": "demo",
					},
				},
			), nil
		},
	}, testVersion()))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(context.Background(), root, []string{"create-resource", "--dry-run", "--query", "request.path"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected custom dry-run to succeed, got %v", err)
	}
	if got := stdout.String(); got != "\"/v1/resources\"\n" {
		t.Fatalf("unexpected output %q", got)
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestMutatingControlPlaneDryRunRendersJSON(t *testing.T) {
	withConfigEnv(t)

	stdout, _, err := executeForTest("db", "create-db-cluster", "--db-cluster-name", "demo-cluster", "--db-cluster-type", "starter", "--project-id", "project-1", "--dry-run")
	if err != nil {
		t.Fatalf("expected dry-run to succeed, got %v", err)
	}

	var got struct {
		DryRun           bool   `json:"dry_run"`
		Command          string `json:"command"`
		Operation        string `json:"operation"`
		WouldSendRequest bool   `json:"would_send_request"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout)
	}
	if !got.DryRun || !got.WouldSendRequest {
		t.Fatalf("unexpected dry-run flags: %+v", got)
	}
	if got.Command != "tdc db create-db-cluster" {
		t.Fatalf("unexpected command %q", got.Command)
	}
	if got.Operation != "create_db_cluster" {
		t.Fatalf("unexpected operation %q", got.Operation)
	}
	if !strings.Contains(stdout, "permission_requirement") || !strings.Contains(stdout, string(authz.StarterClusterCreate)) {
		t.Fatalf("dry-run output did not include permission requirement:\n%s", stdout)
	}
}

func TestRegionOverrideWinsOverEnvironmentCredentials(t *testing.T) {
	t.Setenv("TDC_REGION_CODE", "aws-us-east-1")
	t.Setenv("TDC_PUBLIC_KEY", "test-public")
	t.Setenv("TDC_PRIVATE_KEY", "test-private")

	stdout, _, err := executeForTest("--region", "aws-ap-southeast-1", "db", "create-db-cluster", "--db-cluster-name", "demo-cluster", "--db-cluster-type", "starter", "--project-id", "project-1", "--dry-run")
	if err != nil {
		t.Fatalf("expected dry-run to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "aws ap-southeast-1") {
		t.Fatalf("expected region override in endpoint check, got:\n%s", stdout)
	}
}

func TestRegionOverrideAllowsEnvironmentCredentialsWithoutEnvRegion(t *testing.T) {
	t.Setenv("TDC_REGION_CODE", "")
	t.Setenv("TDC_PUBLIC_KEY", "test-public")
	t.Setenv("TDC_PRIVATE_KEY", "test-private")

	stdout, _, err := executeForTest("--region", "ali-ap-southeast-1", "db", "create-db-cluster", "--db-cluster-name", "demo-cluster", "--db-cluster-type", "starter", "--project-id", "project-1", "--dry-run")
	if err != nil {
		t.Fatalf("expected dry-run to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "alibaba_cloud ap-southeast-1") {
		t.Fatalf("expected region override in endpoint check, got:\n%s", stdout)
	}
}

func TestExplicitEmptyRegionFails(t *testing.T) {
	_, _, err := executeForTest("--region", "", "db", "list-db-clusters")
	if err == nil {
		t.Fatal("expected empty region to fail")
	}
	if got := apperr.ExitCodeFor(err); got != 2 {
		t.Fatalf("expected exit code 2, got %d", got)
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "--region cannot be empty") {
		t.Fatalf("unexpected message %q", got)
	}
}

func TestMutatingControlPlaneDryRunSupportsTextOutput(t *testing.T) {
	withConfigEnv(t)

	stdout, _, err := executeForTest("db", "create-db-cluster", "--db-cluster-name", "demo-cluster", "--db-cluster-type", "starter", "--project-id", "project-1", "--dry-run", "--output", "text")
	if err != nil {
		t.Fatalf("expected dry-run to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "Dry run: tdc db create-db-cluster") {
		t.Fatalf("unexpected text output:\n%s", stdout)
	}
}

func TestQueryAppliesToDryRunResult(t *testing.T) {
	withConfigEnv(t)

	stdout, _, err := executeForTest("db", "create-db-cluster", "--db-cluster-name", "demo-cluster", "--db-cluster-type", "starter", "--project-id", "project-1", "--dry-run", "--query", "command")
	if err != nil {
		t.Fatalf("expected query to succeed, got %v", err)
	}
	if got := stdout; got != "\"tdc db create-db-cluster\"\n" {
		t.Fatalf("unexpected query output %q", got)
	}
}

func TestInvalidQueryFails(t *testing.T) {
	withConfigEnv(t)

	_, _, err := executeForTest("db", "create-db-cluster", "--db-cluster-name", "demo-cluster", "--db-cluster-type", "starter", "--project-id", "project-1", "--dry-run", "--query", "command[")
	if err == nil {
		t.Fatal("expected invalid query to fail")
	}
	if got := apperr.ExitCodeFor(err); got != 2 {
		t.Fatalf("expected exit code 2, got %d", got)
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "invalid --query expression") {
		t.Fatalf("unexpected message %q", got)
	}
}

func TestDryRunRequiresConfigAndCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TDC_REGION_CODE", "")
	t.Setenv("TDC_PUBLIC_KEY", "")
	t.Setenv("TDC_PRIVATE_KEY", "")
	writeConfigOnlyProfile(t, "default")

	_, _, err := executeForTest("db", "create-db-cluster", "--db-cluster-name", "demo-cluster", "--db-cluster-type", "starter", "--project-id", "project-1", "--dry-run")
	if err == nil {
		t.Fatal("expected missing config to fail")
	}
	if got := apperr.ExitCodeFor(err); got != 3 {
		t.Fatalf("expected exit code 3, got %d", got)
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "authentication required") {
		t.Fatalf("unexpected message %q", got)
	}
}

func TestTDCProfileEnvironmentSelectsFileProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TDC_PROFILE", "stage")
	t.Setenv("TDC_REGION_CODE", "")
	t.Setenv("TDC_PUBLIC_KEY", "")
	t.Setenv("TDC_PRIVATE_KEY", "")
	writeCompleteProfile(t, home, "stage")

	stdout, _, err := executeForTest("db", "create-db-cluster", "--db-cluster-name", "demo-cluster", "--db-cluster-type", "starter", "--project-id", "project-1", "--dry-run")
	if err != nil {
		t.Fatalf("expected dry-run to succeed, got %v", err)
	}
	if !strings.Contains(stdout, `profile \"stage\" loaded`) {
		t.Fatalf("expected TDC_PROFILE to select stage profile, got:\n%s", stdout)
	}
}

func TestExplicitEmptyProfileFails(t *testing.T) {
	t.Setenv("TDC_PROFILE", "stage")

	_, _, err := executeForTest("db", "list-db-clusters", "--profile", "")
	if err == nil {
		t.Fatal("expected empty profile to fail")
	}
	if got := apperr.ExitCodeFor(err); got != 2 {
		t.Fatalf("expected exit code 2, got %d", got)
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "--profile cannot be empty") {
		t.Fatalf("unexpected message %q", got)
	}
}

func TestDryRunRejectedOnReadOnlyCommand(t *testing.T) {
	_, _, err := executeForTest("db", "list-db-clusters", "--dry-run")
	if err == nil {
		t.Fatal("expected dry-run on read-only command to fail")
	}
	if got := apperr.ExitCodeFor(err); got != 2 {
		t.Fatalf("expected exit code 2, got %d", got)
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "unknown flag: --dry-run") {
		t.Fatalf("unexpected message %q", got)
	}
}

func withConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TDC_REGION_CODE", "aws-us-east-1")
	t.Setenv("TDC_PUBLIC_KEY", "test-public")
	t.Setenv("TDC_PRIVATE_KEY", "test-private")
}

func writeCompleteProfile(t *testing.T, home, profileName string) {
	t.Helper()
	err := store.WriteProfile(home, profileName, store.ConfigProfile{
		RegionCode: "aws-us-east-1",
	}, store.CredentialsProfile{
		TDCPublicKey:  "test-public",
		TDCPrivateKey: "test-private",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writeConfigOnlyProfile(t *testing.T, profileName string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	err := store.WriteProfile(home, profileName, store.ConfigProfile{
		RegionCode: "aws-us-east-1",
	}, store.CredentialsProfile{})
	if err != nil {
		t.Fatal(err)
	}
}

func executeForTest(args ...string) (string, string, error) {
	if _, ok := os.LookupEnv("TDC_LOGGING"); !ok {
		_ = os.Setenv("TDC_LOGGING", "off")
		defer os.Unsetenv("TDC_LOGGING")
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := NewRootCommand(testVersion())
	err := Execute(context.Background(), root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func testVersion() version.Info {
	return version.Info{
		Version:        "0.0.0-test",
		Commit:         "testcommit",
		Date:           "2026-07-02",
		OS:             "linux",
		Arch:           "amd64",
		InstallSource:  "local",
		ReleaseChannel: "stable",
	}
}
