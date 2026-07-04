package mountstate

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestWriteReadAndRemoveState(t *testing.T) {
	home := t.TempDir()
	mountPath := filepath.Join(home, "mount")
	if err := os.MkdirAll(mountPath, 0o755); err != nil {
		t.Fatal(err)
	}
	state, err := New("stage", "workspace", mountPath, "/", "webdav", "https://fs.test", 1234, true, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	path, err := Write(home, state)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(home, ".tdc", "mounts") {
		t.Fatalf("unexpected state path %q", path)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("state mode: want 0600, got %o", info.Mode().Perm())
		}
	}
	got, gotPath, err := Read(home, mountPath)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if gotPath != path || got.Profile != "stage" || got.FileSystemName != "workspace" || !got.ReadOnly || got.PID != 1234 {
		t.Fatalf("unexpected state %#v path %q", got, gotPath)
	}
	if err := Remove(home, mountPath); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected state removed, got %v", err)
	}
}

func TestControlSocketPathUsesUserRuntimeNamespace(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	mountPath := filepath.Join(t.TempDir(), "mount")
	if err := os.MkdirAll(mountPath, 0o755); err != nil {
		t.Fatal(err)
	}

	path, err := ControlSocketPath(mountPath)
	if err != nil {
		t.Fatalf("ControlSocketPath failed: %v", err)
	}
	if filepath.Dir(path) != runtimeDir {
		t.Fatalf("ControlSocketPath = %q, want under runtime dir %q", path, runtimeDir)
	}
	if filepath.Ext(path) != ".sock" {
		t.Fatalf("ControlSocketPath = %q, want .sock suffix", path)
	}
	again, err := ControlSocketPath(mountPath)
	if err != nil {
		t.Fatalf("ControlSocketPath second call failed: %v", err)
	}
	if again != path {
		t.Fatalf("ControlSocketPath unstable: first %q second %q", path, again)
	}
}

func TestControlSocketPathFallbackIsUIDScoped(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	path, err := ControlSocketPath(filepath.Join(t.TempDir(), "mount"))
	if err != nil {
		t.Fatalf("ControlSocketPath failed: %v", err)
	}
	if got := filepath.Base(filepath.Dir(path)); got != "tdc-"+strconv.Itoa(os.Getuid()) {
		t.Fatalf("ControlSocketPath dir = %q, want uid scoped tdc dir", filepath.Dir(path))
	}
}

func TestCanonicalMountPathRejectsEmpty(t *testing.T) {
	if _, err := CanonicalMountPath(""); err == nil {
		t.Fatal("expected empty mount path to fail")
	}
}
