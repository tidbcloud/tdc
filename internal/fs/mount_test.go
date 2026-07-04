package fs

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Icemap/tdc/internal/fs/mountcontrol"
	"github.com/Icemap/tdc/internal/fs/mountstate"
)

func TestMountCacheDirUsesMountIdentity(t *testing.T) {
	identity := MountCacheIdentity{
		Profile:           "default",
		FileSystemName:    "workspace",
		TenantID:          "tenant-1",
		Endpoint:          "https://fs.example.test",
		RemotePath:        "/",
		MountPath:         "/tmp/tdc-fs",
		APIKeyFingerprint: "key-a",
	}
	first, err := mountCacheDir("/home/user", identity, "")
	if err != nil {
		t.Fatalf("mount cache dir: %v", err)
	}

	identity.APIKeyFingerprint = "key-b"
	second, err := mountCacheDir("/home/user", identity, "")
	if err != nil {
		t.Fatalf("mount cache dir: %v", err)
	}
	if first == second {
		t.Fatalf("expected cache dir to change with identity: %q", first)
	}
}

func TestMountCacheDirHonorsExplicitPath(t *testing.T) {
	got, err := mountCacheDir("/home/user", MountCacheIdentity{}, "./cache")
	if err != nil {
		t.Fatalf("mount cache dir: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("expected explicit cache dir to become absolute, got %q", got)
	}
}

func TestDrainFileSystemRejectsNonFuseMount(t *testing.T) {
	home := t.TempDir()
	mountPath := filepath.Join(home, "mount")
	if err := os.MkdirAll(mountPath, 0o755); err != nil {
		t.Fatal(err)
	}
	state, err := mountstate.New("stage", "workspace", mountPath, "/", "webdav", "https://fs.test", 1234, false, time.Now())
	if err != nil {
		t.Fatalf("mount state: %v", err)
	}
	if _, err := mountstate.Write(home, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	_, err = (Service{HomeDir: home}).DrainFileSystem(context.Background(), DrainFileSystemOptions{MountPath: mountPath, Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "only supports FUSE mounts") {
		t.Fatalf("DrainFileSystem error = %v", err)
	}
}

func TestDrainFileSystemRejectsMissingControlSocket(t *testing.T) {
	home := t.TempDir()
	mountPath := filepath.Join(home, "mount")
	if err := os.MkdirAll(mountPath, 0o755); err != nil {
		t.Fatal(err)
	}
	state, err := mountstate.New("stage", "workspace", mountPath, "/", "fuse", "https://fs.test", os.Getpid(), false, time.Now())
	if err != nil {
		t.Fatalf("mount state: %v", err)
	}
	if _, err := mountstate.Write(home, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	_, err = (Service{HomeDir: home}).DrainFileSystem(context.Background(), DrainFileSystemOptions{MountPath: mountPath, Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "does not expose a control socket") {
		t.Fatalf("DrainFileSystem error = %v", err)
	}
}

func TestDrainFileSystemRequestsControlSocket(t *testing.T) {
	home := t.TempDir()
	mountPath := filepath.Join(home, "mount")
	if err := os.MkdirAll(mountPath, 0o755); err != nil {
		t.Fatal(err)
	}
	socketPath, err := mountstate.ControlSocketPath(mountPath)
	if err != nil {
		t.Fatalf("control socket path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		t.Fatalf("create socket dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var req mountcontrol.DrainRequest
		if err := json.NewDecoder(conn).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if req.TimeoutMS != 2000 {
			t.Errorf("timeout_ms = %d, want 2000", req.TimeoutMS)
		}
		resp := mountcontrol.NewDrainResponse(mountPath, time.Now().UTC())
		resp.Pending.OpenHandles = 1
		resp.Finish(time.Now().UTC())
		_ = json.NewEncoder(conn).Encode(resp)
	}()

	state, err := mountstate.New("stage", "workspace", mountPath, "/", "fuse", "https://fs.test", os.Getpid(), false, time.Now())
	if err != nil {
		t.Fatalf("mount state: %v", err)
	}
	state.ControlSocket = socketPath
	if _, err := mountstate.Write(home, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	result, err := (Service{HomeDir: home}).DrainFileSystem(context.Background(), DrainFileSystemOptions{MountPath: mountPath, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("DrainFileSystem failed: %v", err)
	}
	if result.Status != "drained" || result.Response == nil || result.Response.Pending.OpenHandles != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	<-done
}

func TestAutoPackAfterUnmountWritesDefaultArchive(t *testing.T) {
	home := t.TempDir()
	mountPath := filepath.Join(home, "mount")
	localRoot := filepath.Join(home, "local")
	if err := os.MkdirAll(filepath.Join(localRoot, "overlay", "repo"), 0o755); err != nil {
		t.Fatalf("mkdir local overlay: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localRoot, "overlay", "repo", "cache.txt"), []byte("cached\n"), 0o644); err != nil {
		t.Fatalf("write overlay file: %v", err)
	}
	if err := os.MkdirAll(mountPath, 0o755); err != nil {
		t.Fatalf("mkdir mount path: %v", err)
	}

	var stored []byte
	var archivePath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/fs/.tdc/packs/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPut || !strings.HasPrefix(r.URL.Path, "/v1/fs/.tdc/packs/") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		archivePath = strings.TrimPrefix(r.URL.Path, "/v1/fs")
		var err error
		stored, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read archive body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"path": archivePath, "size": len(stored)})
	}))
	defer server.Close()

	state, err := mountstate.New("stage", "workspace", mountPath, "/workspace", "fuse", server.URL, os.Getpid(), false, time.Now())
	if err != nil {
		t.Fatalf("mount state: %v", err)
	}
	state.LocalRoot = localRoot
	state.MountProfile = portableMountProfile
	state.PackPaths = []string{"/"}
	if _, err := mountstate.Write(home, state); err != nil {
		t.Fatalf("write mount state: %v", err)
	}

	message, err := testService(home, server.URL).autoPackAfterUnmount(context.Background(), UnmountFileSystemOptions{Profile: dataProfile()}, state, mountPath)
	if err != nil {
		t.Fatalf("autoPackAfterUnmount failed: %v", err)
	}
	if !strings.Contains(message, "packed") || archivePath == "" || len(stored) == 0 {
		t.Fatalf("unexpected auto pack result message=%q archive=%q size=%d", message, archivePath, len(stored))
	}
	restoreRoot := filepath.Join(home, "restore")
	if _, err := extractPackArchive(context.Background(), bytes.NewReader(stored), unpackArchiveOptions{LocalRoot: restoreRoot, Replace: true}); err != nil {
		t.Fatalf("extract stored archive: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(restoreRoot, "overlay", "repo", "cache.txt"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(got) != "cached\n" {
		t.Fatalf("restored content = %q", got)
	}
}

func TestAutoUnpackMountRestoresDefaultArchive(t *testing.T) {
	home := t.TempDir()
	sourceRoot := filepath.Join(home, "source")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "overlay", "repo"), 0o755); err != nil {
		t.Fatalf("mkdir source overlay: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "overlay", "repo", "cache.txt"), []byte("cached\n"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	var archive bytes.Buffer
	if _, err := writePackArchive(context.Background(), &archive, packArchiveOptions{
		LocalRoot:            sourceRoot,
		RemoteRoot:           "/workspace",
		MountProfile:         portableMountProfile,
		ProfilePackPaths:     []string{"/"},
		PreviousReplacePaths: []string{"/"},
	}); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	defaultArchivePath, err := defaultPackArchivePath("/workspace", portableMountProfile)
	if err != nil {
		t.Fatalf("default archive path: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.TrimPrefix(r.URL.Path, "/v1/fs") != defaultArchivePath {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()

	service := testService(home, server.URL)
	client, err := service.dataClient(dataProfile(), "", "auto unpack test")
	if err != nil {
		t.Fatalf("create data client: %v", err)
	}
	targetRoot := filepath.Join(home, "target")
	message, err := service.autoUnpackMount(context.Background(), mountInputs{
		client:       client,
		localRoot:    targetRoot,
		remotePath:   "/workspace",
		mountProfile: portableMountProfile,
		packPaths:    []string{"/"},
	})
	if err != nil {
		t.Fatalf("autoUnpackMount failed: %v", err)
	}
	if !strings.Contains(message, "restored") {
		t.Fatalf("message = %q", message)
	}
	got, err := os.ReadFile(filepath.Join(targetRoot, "overlay", "repo", "cache.txt"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(got) != "cached\n" {
		t.Fatalf("restored content = %q", got)
	}
}
