//go:build !windows

package fs

import (
	"bytes"
	"testing"
	"time"
)

func TestFuseReadCacheStoresAndInvalidates(t *testing.T) {
	cache := newFuseReadCache(16, 16, time.Minute)
	version := fuseObjectVersion{Revision: 1, ResourceID: "file-a"}
	cache.put("/a.txt", []byte("alpha"), version)

	got, ok := cache.get("/a.txt", version)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !bytes.Equal(got, []byte("alpha")) {
		t.Fatalf("unexpected cache data %q", got)
	}

	got[0] = 'x'
	again, ok := cache.get("/a.txt", version)
	if !ok {
		t.Fatal("expected cache hit after caller mutation")
	}
	if string(again) != "alpha" {
		t.Fatalf("cache returned mutable backing data %q", again)
	}

	cache.invalidate("/a.txt")
	if _, ok := cache.get("/a.txt", version); ok {
		t.Fatal("expected cache miss after invalidate")
	}
}

func TestFuseReadCacheMissesVersionMismatch(t *testing.T) {
	cache := newFuseReadCache(16, 16, time.Minute)
	cache.put("/a.txt", []byte("alpha"), fuseObjectVersion{Revision: 1, ResourceID: "file-a"})

	if _, ok := cache.get("/a.txt", fuseObjectVersion{Revision: 2, ResourceID: "file-a"}); ok {
		t.Fatal("expected cache miss when revision changes")
	}
	if _, ok := cache.get("/a.txt", fuseObjectVersion{Revision: 1, ResourceID: "file-b"}); ok {
		t.Fatal("expected cache miss when resource id changes")
	}
	if got, ok := cache.get("/a.txt", fuseObjectVersion{}); !ok || string(got) != "alpha" {
		t.Fatalf("expected cache hit when caller has no version, ok=%v got=%q", ok, got)
	}
}

func TestFuseReadCacheEvictsBySizeAndTTL(t *testing.T) {
	cache := newFuseReadCache(5, 16, time.Minute)
	cache.put("/a.txt", []byte("aaaa"), fuseObjectVersion{})
	cache.put("/b.txt", []byte("bbbb"), fuseObjectVersion{})
	if _, ok := cache.get("/a.txt", fuseObjectVersion{}); ok {
		t.Fatal("expected oldest entry to be evicted by aggregate size")
	}
	if _, ok := cache.get("/b.txt", fuseObjectVersion{}); !ok {
		t.Fatal("expected newest entry to remain cached")
	}

	expiring := newFuseReadCache(16, 16, time.Nanosecond)
	expiring.put("/c.txt", []byte("c"), fuseObjectVersion{})
	time.Sleep(time.Millisecond)
	if _, ok := expiring.get("/c.txt", fuseObjectVersion{}); ok {
		t.Fatal("expected cache miss after TTL")
	}
}
