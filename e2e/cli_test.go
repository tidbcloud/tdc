package e2e

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHelpAndVersion(t *testing.T) {
	bin := tdcBinary(t)

	root := runTDC(t, bin, "help")
	root.wantExitCode(0)
	root.wantStdoutContains("Commands:")
	root.wantStdoutContains("db")
	root.wantStdoutNotContains("-h,")

	db := runTDC(t, bin, "db", "help")
	db.wantExitCode(0)
	db.wantStdoutContains("create-db-cluster")
	db.wantStdoutContains("prepare-db-query-access")
	db.wantStdoutContains("create-db-connection-string")

	subcommand := runTDC(t, bin, "fs", "mount-file-system", "help")
	subcommand.wantExitCode(0)
	subcommand.wantStdoutContains("Mount a tdc fs resource locally.")
	subcommand.wantStdoutContains("--mount-path")
	subcommand.wantStdoutContains("--foreground")
	subcommand.wantStdoutContains("--mount-profile")
	subcommand.wantStdoutContains("--local-root")
	subcommand.wantStdoutContains("--pack-path")

	copyFile := runTDC(t, bin, "fs", "copy-file", "help")
	copyFile.wantExitCode(0)
	copyFile.wantStdoutContains("--from-local")
	copyFile.wantStdoutContains("--to-remote")
	copyFile.wantStdoutContains("--from-stdin")
	copyFile.wantStdoutContains("--to-stdout")
	copyFile.wantStdoutContains("--description")

	chmodFile := runTDC(t, bin, "fs", "chmod-file", "help")
	chmodFile.wantExitCode(0)
	chmodFile.wantStdoutContains("--mode")

	packFileSystem := runTDC(t, bin, "fs", "pack-file-system", "help")
	packFileSystem.wantExitCode(0)
	packFileSystem.wantStdoutContains("--archive-path")
	packFileSystem.wantStdoutContains("--mount-profile")
	packFileSystem.wantStdoutContains("--path")

	gitClone := runTDC(t, bin, "git", "clone-git-workspace", "help")
	gitClone.wantExitCode(0)
	gitClone.wantStdoutContains("--repo-url")
	gitClone.wantStdoutContains("--target-path")
	gitClone.wantStdoutContains("--hydrate")

	version := runTDC(t, bin, "fs", "mount-file-system", "--version")
	version.wantExitCode(0)
	version.wantStdoutContains("tdc ")
}

func TestErrorsAreRenderedAtCLIBoundary(t *testing.T) {
	bin := tdcBinary(t)

	shortFlag := runTDC(t, bin, "-h")
	shortFlag.wantExitCode(2)
	shortFlag.wantStderrContains("tdc [ERROR]: short flags are not supported")

	unknown := runTDC(t, bin, "db", "missing-command")
	unknown.wantExitCode(2)
	unknown.wantStderrContains(`tdc [ERROR]: unknown command "missing-command" for "tdc db"`)

	releaseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		artifact := artifactNameForRuntime(t)
		fmt.Fprintf(w, `{
			"tag_name": "v99.0.0",
			"html_url": "https://github.com/Icemap/tdc/releases/tag/v99.0.0",
			"assets": [
				{
					"name": %q,
					"browser_download_url": "https://github.com/Icemap/tdc/releases/download/v99.0.0/%s"
				}
			]
		}`, artifact, artifact)
	}))
	defer releaseServer.Close()

	checkUpdate := runTDCWithInput(t, bin, "", []string{"TDC_RELEASE_API_BASE_URL=" + releaseServer.URL}, "cli", "check-update", "--query", "latest_version")
	checkUpdate.wantExitCode(0)
	checkUpdate.wantStdoutContains(`"99.0.0"`)

	update := runTDC(t, bin, "cli", "update", "--dry-run")
	update.wantExitCode(1)
	update.wantStderrContains("tdc [ERROR]: tdc install source")

	invalidQuery := runTDCWithInput(t, bin, "", tdcConfigEnv(), append(createClusterDryRunArgs(), "--query", "command[")...)
	invalidQuery.wantExitCode(2)
	invalidQuery.wantStderrContains("tdc [ERROR]: invalid --query expression")
}

func TestOutputQueryAndDryRun(t *testing.T) {
	bin := tdcBinary(t)
	env := tdcConfigEnv()

	dryRun := runTDCWithInput(t, bin, "", env, createClusterDryRunArgs()...)
	dryRun.wantExitCode(0)
	dryRun.wantStdoutContains(`"dry_run": true`)
	dryRun.wantStdoutContains(`"would_send_request": true`)

	human := runTDCWithInput(t, bin, "", env, append(createClusterDryRunArgs(), "--output", "human")...)
	human.wantExitCode(0)
	human.wantStdoutContains("Dry run: tdc db create-db-cluster")

	query := runTDCWithInput(t, bin, "", env, append(createClusterDryRunArgs(), "--query", "command")...)
	query.wantExitCode(0)
	query.wantStdoutContains(`"tdc db create-db-cluster"`)

	readOnlyDryRun := runTDC(t, bin, "db", "list-db-clusters", "--dry-run")
	readOnlyDryRun.wantExitCode(2)
	readOnlyDryRun.wantStderrContains("tdc [ERROR]: invalid flag for tdc db list-db-clusters: unknown flag: --dry-run")
}

func tdcConfigEnv() []string {
	return []string{
		"TDC_CLOUD_PROVIDER=aws",
		"TDC_REGION_CODE=us-east-1",
		"TDC_PUBLIC_KEY=e2e-public",
		"TDC_PRIVATE_KEY=e2e-private",
	}
}

func createClusterDryRunArgs() []string {
	return []string{
		"db", "create-db-cluster",
		"--db-cluster-name", "demo-cluster",
		"--db-cluster-type", "starter",
		"--project-id", "project-1",
		"--dry-run",
	}
}

func artifactNameForRuntime(t *testing.T) string {
	t.Helper()
	switch runtime.GOOS {
	case "darwin", "linux":
		if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
			t.Skipf("unsupported release target %s/%s", runtime.GOOS, runtime.GOARCH)
		}
		return fmt.Sprintf("tdc_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	case "windows":
		if runtime.GOARCH != "amd64" {
			t.Skipf("unsupported release target %s/%s", runtime.GOOS, runtime.GOARCH)
		}
		return "tdc_windows_amd64.zip"
	default:
		t.Skipf("unsupported release target %s/%s", runtime.GOOS, runtime.GOARCH)
		return ""
	}
}

func TestConfigureWritesLocalProfile(t *testing.T) {
	bin := tdcBinary(t)
	home := t.TempDir()

	result := runTDCWithInput(t, bin, "aws\nus-east-1\npublic-key\nprivate-key\n", []string{"HOME=" + home}, "configure", "--profile", "stage")
	result.wantExitCode(0)
	result.wantStdoutContains(`Profile "stage" configured.`)
	result.wantStdoutNotContains("private-key")

	configBytes, err := os.ReadFile(filepath.Join(home, ".tdc", "config"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	credentialsPath := filepath.Join(home, ".tdc", "credentials")
	credentialsBytes, err := os.ReadFile(credentialsPath)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}

	if !strings.Contains(string(configBytes), `[stage]`) ||
		!strings.Contains(string(configBytes), `cloud_provider = 'aws'`) ||
		!strings.Contains(string(configBytes), `region_code = 'us-east-1'`) {
		t.Fatalf("config did not contain expected stage profile:\n%s", string(configBytes))
	}
	if !strings.Contains(string(credentialsBytes), `tdc_public_key = 'public-key'`) ||
		!strings.Contains(string(credentialsBytes), `tdc_private_key = 'private-key'`) {
		t.Fatalf("credentials did not contain expected keys:\n%s", string(credentialsBytes))
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(credentialsPath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("credentials mode: want 0600, got %o", info.Mode().Perm())
		}
	}
}

func TestConfigureNonInteractiveFromEnvironment(t *testing.T) {
	bin := tdcBinary(t)
	home := t.TempDir()

	result := runTDCWithInput(t, bin, "", []string{
		"HOME=" + home,
		"TDC_CLOUD_PROVIDER=aws",
		"TDC_REGION_CODE=us-east-1",
		"TDC_PUBLIC_KEY=ci-public",
		"TDC_PRIVATE_KEY=ci-private",
	}, "configure", "--profile", "ci", "--non-interactive")
	result.wantExitCode(0)
	result.wantStdoutContains(`Profile "ci" configured.`)
	result.wantStdoutNotContains("ci-private")

	configBytes, err := os.ReadFile(filepath.Join(home, ".tdc", "config"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	credentialsBytes, err := os.ReadFile(filepath.Join(home, ".tdc", "credentials"))
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	if !strings.Contains(string(configBytes), `[ci]`) ||
		!strings.Contains(string(configBytes), `cloud_provider = 'aws'`) {
		t.Fatalf("config did not contain expected ci profile:\n%s", string(configBytes))
	}
	if !strings.Contains(string(credentialsBytes), `tdc_public_key = 'ci-public'`) ||
		!strings.Contains(string(credentialsBytes), `tdc_private_key = 'ci-private'`) {
		t.Fatalf("credentials did not contain expected ci keys:\n%s", string(credentialsBytes))
	}
}

func tdcBinary(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("TDC_E2E_BIN")
	if bin == "" {
		t.Skip("TDC_E2E_BIN is not set; run make e2e")
	}
	return bin
}

func runTDC(t *testing.T, bin string, args ...string) commandResult {
	t.Helper()
	return runTDCWithInput(t, bin, "", nil, args...)
}

func runTDCWithInput(t *testing.T, bin, stdin string, env []string, args ...string) commandResult {
	t.Helper()

	cmd := exec.Command(bin, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("failed to run %s %s: %v", bin, strings.Join(args, " "), err)
		}
		exitCode = exitErr.ExitCode()
	}

	return commandResult{
		t:        t,
		args:     args,
		exitCode: exitCode,
		stdout:   stdout.String(),
		stderr:   stderr.String(),
	}
}

type commandResult struct {
	t        *testing.T
	args     []string
	exitCode int
	stdout   string
	stderr   string
}

func (r commandResult) wantExitCode(want int) {
	r.t.Helper()
	if r.exitCode != want {
		r.fail("exit code: want %d, got %d", want, r.exitCode)
	}
}

func (r commandResult) wantStdoutContains(want string) {
	r.t.Helper()
	if !strings.Contains(r.stdout, want) {
		r.fail("stdout should contain %q", want)
	}
}

func (r commandResult) wantStdoutNotContains(want string) {
	r.t.Helper()
	if strings.Contains(r.stdout, want) {
		r.fail("stdout should not contain %q", want)
	}
}

func (r commandResult) wantStderrContains(want string) {
	r.t.Helper()
	if !strings.Contains(r.stderr, want) {
		r.fail("stderr should contain %q", want)
	}
}

func (r commandResult) fail(format string, args ...any) {
	r.t.Helper()
	message := fmt.Sprintf(format, args...)
	r.t.Fatalf("%s", strings.Join([]string{
		"command: tdc " + strings.Join(r.args, " "),
		message,
		"stdout:\n" + r.stdout,
		"stderr:\n" + r.stderr,
	}, "\n"))
}
