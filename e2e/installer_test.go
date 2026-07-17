package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUnixInstallerDefaultsToTDCBin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell installer is not supported on Windows")
	}
	script := filepath.Join("..", "scripts", "install.sh")
	syntax := exec.Command("sh", "-n", script)
	if output, err := syntax.CombinedOutput(); err != nil {
		t.Fatalf("install.sh syntax check failed: %v\n%s", err, output)
	}

	home := t.TempDir()
	cmd := exec.Command("sh", script, "--dry-run")
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh --dry-run failed: %v\n%s", err, output)
	}
	got := string(output)
	installDir := filepath.Join(home, ".tdc", "bin")
	for _, want := range []string{
		"target: " + filepath.Join(installDir, "tdc"),
		"companion_target: " + filepath.Join(installDir, "tdc-drive9"),
		`path_export: export PATH="` + installDir + `:$PATH"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("installer dry-run should contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "/usr/local/bin") {
		t.Fatalf("installer dry-run should not target /usr/local/bin:\n%s", got)
	}
}

func TestInstallersDoNotEscalatePrivileges(t *testing.T) {
	shellBytes, err := os.ReadFile(filepath.Join("..", "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	shell := string(shellBytes)
	if strings.Contains(shell, "sudo") || strings.Contains(shell, "/usr/local/bin") {
		t.Fatalf("install.sh must remain user-owned and privilege-free")
	}

	powerShellBytes, err := os.ReadFile(filepath.Join("..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	powerShell := string(powerShellBytes)
	if !strings.Contains(powerShell, `$DefaultInstallDir = Join-Path (Join-Path $HOME ".tdc") "bin"`) {
		t.Fatalf("install.ps1 should default to $HOME\\.tdc\\bin")
	}
}
