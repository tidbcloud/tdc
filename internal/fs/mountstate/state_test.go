package mountstate

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestCanonicalMountPathRejectsEmpty(t *testing.T) {
	if _, err := CanonicalMountPath(""); err == nil {
		t.Fatal("expected empty mount path to fail")
	}
}
