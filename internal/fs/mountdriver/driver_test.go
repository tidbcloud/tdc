package mountdriver

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

func TestResolveAutoPrefersFUSEWhenAvailable(t *testing.T) {
	driver, err := ResolveWithDeps("auto", "darwin", fakeLookPath(nil), fakeStat(map[string]bool{
		"/Library/Filesystems/macfuse.fs/Contents/Resources/mount_macfuse": true,
	}), exec.CommandContext)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if driver.Name() != "fuse" {
		t.Fatalf("unexpected driver %q", driver.Name())
	}
}

func TestResolveAutoFallsBackToWebDAVWhenFUSEUnavailable(t *testing.T) {
	driver, err := ResolveWithDeps("auto", "darwin", fakeLookPath(map[string]bool{
		"mount_webdav": true,
		"umount":       true,
	}), fakeStat(nil), exec.CommandContext)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if driver.Name() != "webdav" {
		t.Fatalf("unexpected driver %q", driver.Name())
	}
}

func TestResolveFUSEDriver(t *testing.T) {
	driver, err := ResolveWithDeps("fuse", "darwin", fakeLookPath(nil), fakeStat(map[string]bool{
		"/Library/Filesystems/macfuse.fs/Contents/Resources/mount_macfuse": true,
	}), exec.CommandContext)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if driver.Name() != "fuse" {
		t.Fatalf("unexpected driver %q", driver.Name())
	}
	if err := driver.CheckPrerequisites(); err != nil {
		t.Fatalf("CheckPrerequisites failed: %v", err)
	}
}

func TestWebDAVPrerequisitesDarwin(t *testing.T) {
	driver := WebDAV{
		GOOS: "darwin",
		LookPath: func(name string) (string, error) {
			if name == "mount_webdav" || name == "umount" {
				return "/usr/sbin/" + name, nil
			}
			return "", exec.ErrNotFound
		},
	}
	if err := driver.CheckPrerequisites(); err != nil {
		t.Fatalf("CheckPrerequisites failed: %v", err)
	}
}

func TestWebDAVMountCommandDarwin(t *testing.T) {
	var gotName string
	var gotArgs []string
	driver := WebDAV{
		GOOS: "darwin",
		LookPath: func(name string) (string, error) {
			return "/usr/sbin/" + name, nil
		},
		CommandContext: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			gotName = name
			gotArgs = append([]string(nil), args...)
			return exec.CommandContext(ctx, "true")
		},
	}
	if err := driver.Mount(context.Background(), "http://127.0.0.1:9999/tdc/", "/tmp/tdc-mount"); err != nil {
		t.Fatalf("Mount failed: %v", err)
	}
	if gotName != "mount_webdav" || !reflect.DeepEqual(gotArgs, []string{"http://127.0.0.1:9999/tdc/", "/tmp/tdc-mount"}) {
		t.Fatalf("unexpected mount command %s %#v", gotName, gotArgs)
	}
}

func TestWebDAVLinuxUnsupported(t *testing.T) {
	driver := WebDAV{GOOS: "linux"}
	if err := driver.CheckPrerequisites(); err == nil {
		t.Fatal("expected linux WebDAV mount to be unsupported")
	}
}

func fakeLookPath(found map[string]bool) func(string) (string, error) {
	return func(name string) (string, error) {
		if found[name] {
			return "/usr/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}
}

func fakeStat(found map[string]bool) func(string) (os.FileInfo, error) {
	return func(name string) (os.FileInfo, error) {
		if found[name] {
			return fakeFileInfo{}, nil
		}
		return nil, os.ErrNotExist
	}
}

type fakeFileInfo struct{}

func (fakeFileInfo) Name() string       { return "fake" }
func (fakeFileInfo) Size() int64        { return 0 }
func (fakeFileInfo) Mode() os.FileMode  { return 0o755 }
func (fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeFileInfo) IsDir() bool        { return false }
func (fakeFileInfo) Sys() any           { return nil }
