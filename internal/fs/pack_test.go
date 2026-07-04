package fs

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	apifs "github.com/Icemap/tdc/internal/api/fs"
)

func TestPackArchiveRoundTripPortableOverlay(t *testing.T) {
	srcRoot := t.TempDir()
	mustWriteTestFile(t, filepath.Join(srcRoot, "overlay", "repo", ".git", "config"), "[core]\n", 0o600)
	mustWriteTestFile(t, filepath.Join(srcRoot, "overlay", "repo", "dist", "app.js"), "console.log('tdc')\n", 0o644)
	mustWriteTestFile(t, filepath.Join(srcRoot, "overlay", "repo", "src", "main.go"), "package main\n", 0o644)
	if runtime.GOOS != "windows" {
		if err := os.Symlink("config", filepath.Join(srcRoot, "overlay", "repo", ".git", "config.link")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	}

	var buf bytes.Buffer
	manifest, err := writePackArchive(context.Background(), &buf, packArchiveOptions{
		LocalRoot:        srcRoot,
		RemoteRoot:       "/workspace",
		MountProfile:     portableMountProfile,
		ProfilePackPaths: []string{"/"},
	})
	if err != nil {
		t.Fatalf("writePackArchive: %v", err)
	}
	if want := []string{"/repo"}; !reflect.DeepEqual(manifest.Paths, want) {
		t.Fatalf("manifest paths = %v, want %v", manifest.Paths, want)
	}
	if want := []string{"/repo"}; !reflect.DeepEqual(manifest.ReplacePaths, want) {
		t.Fatalf("manifest replace paths = %v, want %v", manifest.ReplacePaths, want)
	}

	dstRoot := t.TempDir()
	mustWriteTestFile(t, filepath.Join(dstRoot, "overlay", "repo", "stale.txt"), "stale\n", 0o644)
	got, err := extractPackArchive(context.Background(), bytes.NewReader(buf.Bytes()), unpackArchiveOptions{
		LocalRoot: dstRoot,
		Replace:   true,
	})
	if err != nil {
		t.Fatalf("extractPackArchive: %v", err)
	}
	if !reflect.DeepEqual(got.Paths, manifest.Paths) {
		t.Fatalf("unpacked paths = %v, want %v", got.Paths, manifest.Paths)
	}
	assertTestFileContent(t, filepath.Join(dstRoot, "overlay", "repo", ".git", "config"), "[core]\n")
	assertTestFileContent(t, filepath.Join(dstRoot, "overlay", "repo", "dist", "app.js"), "console.log('tdc')\n")
	assertTestFileContent(t, filepath.Join(dstRoot, "overlay", "repo", "src", "main.go"), "package main\n")
	if _, err := os.Lstat(filepath.Join(dstRoot, "overlay", "repo", "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale file still exists after replace: err=%v", err)
	}
	if runtime.GOOS != "windows" {
		target, err := os.Readlink(filepath.Join(dstRoot, "overlay", "repo", ".git", "config.link"))
		if err != nil {
			t.Fatalf("read symlink: %v", err)
		}
		if target != "config" {
			t.Fatalf("symlink target = %q, want config", target)
		}
	}
}

func TestPackArchiveCodingAgentNamedPathsAndTombstones(t *testing.T) {
	srcRoot := t.TempDir()
	mustWriteTestFile(t, filepath.Join(srcRoot, "overlay", "repo", ".git", "config"), "git\n", 0o644)
	mustWriteTestFile(t, filepath.Join(srcRoot, "overlay", "repo", "src", "main.go"), "package main\n", 0o644)

	var buf bytes.Buffer
	manifest, err := writePackArchive(context.Background(), &buf, packArchiveOptions{
		LocalRoot:            srcRoot,
		RemoteRoot:           "/workspace",
		LocalPrefix:          "repo",
		MountProfile:         defaultMountProfile,
		ProfilePackPaths:     []string{".git", "dist"},
		PreviousReplacePaths: []string{"/repo/node_modules"},
	})
	if err != nil {
		t.Fatalf("writePackArchive: %v", err)
	}
	if want := []string{"/repo/.git"}; !reflect.DeepEqual(manifest.Paths, want) {
		t.Fatalf("manifest paths = %v, want %v", manifest.Paths, want)
	}
	wantReplace := []string{"/repo/.git", "/repo/dist", "/repo/node_modules"}
	if !reflect.DeepEqual(manifest.ReplacePaths, wantReplace) {
		t.Fatalf("replace paths = %v, want %v", manifest.ReplacePaths, wantReplace)
	}

	dstRoot := t.TempDir()
	mustWriteTestFile(t, filepath.Join(dstRoot, "overlay", "repo", "dist", "old.js"), "stale\n", 0o644)
	if _, err := extractPackArchive(context.Background(), bytes.NewReader(buf.Bytes()), unpackArchiveOptions{
		LocalRoot: dstRoot,
		Replace:   true,
	}); err != nil {
		t.Fatalf("extractPackArchive: %v", err)
	}
	assertTestFileContent(t, filepath.Join(dstRoot, "overlay", "repo", ".git", "config"), "git\n")
	if _, err := os.Lstat(filepath.Join(dstRoot, "overlay", "repo", "dist")); !os.IsNotExist(err) {
		t.Fatalf("dist tombstone still exists after replace: err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(dstRoot, "overlay", "repo", "src", "main.go")); !os.IsNotExist(err) {
		t.Fatalf("remote-managed source file was unexpectedly packed: err=%v", err)
	}
}

func TestPackAndUnpackFileSystemUseRemoteArchive(t *testing.T) {
	profile := dataProfile()
	srcRoot := t.TempDir()
	mustWriteTestFile(t, filepath.Join(srcRoot, "overlay", "repo", "cache", "item.txt"), "portable cache\n", 0o644)

	var stored []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/packs/test.tar.gz":
			if len(stored) == 0 {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(stored)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/fs/packs/test.tar.gz":
			if got := r.Header.Values("X-Dat9-Tag"); strings.Join(got, ",") != "tdc.pack.format=tdc.pack.v1,tdc.pack.profile=portable" {
				t.Fatalf("X-Dat9-Tag = %v", got)
			}
			var err error
			stored, err = io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(apifs.WriteResponse{Revision: 7})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	service := testService(t.TempDir(), server.URL)
	packResult, err := service.PackFileSystem(context.Background(), PackFileSystemOptions{
		Profile:      profile,
		LocalRoot:    srcRoot,
		RemoteRoot:   "/workspace",
		MountProfile: portableMountProfile,
		ArchivePath:  "/packs/test.tar.gz",
	})
	if err != nil {
		t.Fatalf("PackFileSystem: %v", err)
	}
	if packResult.Status != "packed" || packResult.ArchivePath != "/packs/test.tar.gz" || len(stored) == 0 {
		t.Fatalf("unexpected pack result: %#v stored=%d", packResult, len(stored))
	}

	dstRoot := t.TempDir()
	unpackResult, err := service.UnpackFileSystem(context.Background(), UnpackFileSystemOptions{
		Profile:      profile,
		LocalRoot:    dstRoot,
		RemoteRoot:   "/workspace",
		MountProfile: portableMountProfile,
		ArchivePath:  "/packs/test.tar.gz",
	})
	if err != nil {
		t.Fatalf("UnpackFileSystem: %v", err)
	}
	if unpackResult.Status != "unpacked" || unpackResult.Entries == 0 {
		t.Fatalf("unexpected unpack result: %#v", unpackResult)
	}
	assertTestFileContent(t, filepath.Join(dstRoot, "overlay", "repo", "cache", "item.txt"), "portable cache\n")
}

func mustWriteTestFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertTestFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}
