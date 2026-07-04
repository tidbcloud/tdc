//go:build !windows

package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFuseWriteBackStoreRecoversPendingWrites(t *testing.T) {
	identity := testMountCacheIdentity()
	store := newFuseWriteBackStore(t.TempDir(), 1<<20, identity)
	baseVersion := fuseObjectVersion{Revision: 7, ResourceID: "file-a"}
	dirtyRanges := []fuseDirtyRange{{Start: 1, End: 3}}
	if err := store.put("/workspace/a.txt", []byte("alpha"), baseVersion, 5, dirtyRanges); err != nil {
		t.Fatalf("put pending write: %v", err)
	}

	var gotPath string
	var gotData string
	var gotVersion fuseObjectVersion
	var gotBaseSize int64
	var gotDirty []fuseDirtyRange
	count, err := store.recover(context.Background(), func(_ context.Context, remotePath string, data []byte, version fuseObjectVersion, baseSize int64, dirty []fuseDirtyRange) (fuseObjectVersion, error) {
		gotPath = remotePath
		gotData = string(data)
		gotVersion = version
		gotBaseSize = baseSize
		gotDirty = append([]fuseDirtyRange(nil), dirty...)
		return version.withRevision(8), nil
	})
	if err != nil {
		t.Fatalf("recover pending write: %v", err)
	}
	if count != 1 || gotPath != "/workspace/a.txt" || gotData != "alpha" {
		t.Fatalf("unexpected recovery count=%d path=%q data=%q", count, gotPath, gotData)
	}
	if gotVersion != baseVersion {
		t.Fatalf("unexpected base version %#v", gotVersion)
	}
	if gotBaseSize != 5 || len(gotDirty) != 1 || gotDirty[0] != dirtyRanges[0] {
		t.Fatalf("unexpected dirty metadata baseSize=%d dirty=%#v", gotBaseSize, gotDirty)
	}
	if _, err := os.Stat(store.metaPath("/workspace/a.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected metadata to be removed, got %v", err)
	}
}

func TestFuseWriteBackStoreKeepsPendingWriteAfterUploadFailure(t *testing.T) {
	store := newFuseWriteBackStore(t.TempDir(), 1<<20, testMountCacheIdentity())
	boom := errors.New("boom")
	_, err := store.putAndUpload(context.Background(), "/workspace/a.txt", []byte("alpha"), fuseObjectVersion{}, 0, nil, func(context.Context, string, []byte, fuseObjectVersion, int64, []fuseDirtyRange) (fuseObjectVersion, error) {
		return fuseObjectVersion{}, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected upload error, got %v", err)
	}
	if _, err := os.Stat(store.dataPath("/workspace/a.txt")); err != nil {
		t.Fatalf("expected pending data to remain: %v", err)
	}
	if filepath.Dir(store.dataPath("/workspace/a.txt")) != store.dir {
		t.Fatalf("pending data path escaped cache dir")
	}
}

func TestFuseWriteBackStoreRejectsDifferentCacheIdentity(t *testing.T) {
	dir := t.TempDir()
	store := newFuseWriteBackStore(dir, 1<<20, testMountCacheIdentity())
	if err := store.put("/workspace/a.txt", []byte("alpha"), fuseObjectVersion{}, 0, nil); err != nil {
		t.Fatalf("put pending write: %v", err)
	}

	other := testMountCacheIdentity()
	other.Profile = "other"
	recoverStore := newFuseWriteBackStore(dir, 1<<20, other)
	_, err := recoverStore.recover(context.Background(), func(context.Context, string, []byte, fuseObjectVersion, int64, []fuseDirtyRange) (fuseObjectVersion, error) {
		t.Fatal("upload should not be called for mismatched identity")
		return fuseObjectVersion{}, nil
	})
	if err == nil {
		t.Fatal("expected cache identity mismatch error")
	}
}

func testMountCacheIdentity() MountCacheIdentity {
	return MountCacheIdentity{
		Profile:           "default",
		FileSystemName:    "workspace",
		TenantID:          "tenant-1",
		Endpoint:          "https://fs.example.test",
		RemotePath:        "/",
		MountPath:         "/tmp/tdc-fs",
		APIKeyFingerprint: "abc123",
	}
}
