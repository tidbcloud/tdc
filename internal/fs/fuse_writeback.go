//go:build !windows

package fs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const defaultFuseWriteBackMaxBytes = 64 << 20

type fuseWriteBackStore struct {
	dir      string
	maxBytes int64
	identity MountCacheIdentity
}

type fuseWriteBackMeta struct {
	Path          string             `json:"path"`
	Size          int64              `json:"size"`
	BaseSize      int64              `json:"base_size,omitempty"`
	Mtime         time.Time          `json:"mtime"`
	CreatedAt     time.Time          `json:"created_at"`
	CacheIdentity MountCacheIdentity `json:"cache_identity"`
	BaseVersion   fuseObjectVersion  `json:"base_version,omitempty"`
	DirtyRanges   []fuseDirtyRange   `json:"dirty_ranges,omitempty"`
}

type fuseDirtyRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type fuseWriteUploadFunc func(context.Context, string, []byte, fuseObjectVersion, int64, []fuseDirtyRange) (fuseObjectVersion, error)

type fuseWriteBackPendingStats struct {
	Count int
	Bytes int64
}

func newFuseWriteBackStore(dir string, maxBytes int64, identity MountCacheIdentity) *fuseWriteBackStore {
	if dir == "" || maxBytes == 0 {
		return nil
	}
	if maxBytes < 0 {
		maxBytes = defaultFuseWriteBackMaxBytes
	}
	return &fuseWriteBackStore{dir: filepath.Join(dir, "pending"), maxBytes: maxBytes, identity: identity}
}

func (s *fuseWriteBackStore) putAndUpload(ctx context.Context, remotePath string, data []byte, baseVersion fuseObjectVersion, baseSize int64, dirtyRanges []fuseDirtyRange, upload fuseWriteUploadFunc) (fuseObjectVersion, error) {
	if s == nil {
		return upload(ctx, remotePath, data, baseVersion, baseSize, dirtyRanges)
	}
	if int64(len(data)) > s.maxBytes {
		return upload(ctx, remotePath, data, baseVersion, baseSize, dirtyRanges)
	}
	if err := s.put(remotePath, data, baseVersion, baseSize, dirtyRanges); err != nil {
		return fuseObjectVersion{}, err
	}
	version, err := upload(ctx, remotePath, data, baseVersion, baseSize, dirtyRanges)
	if err != nil {
		return fuseObjectVersion{}, err
	}
	return version, s.remove(remotePath)
}

func (s *fuseWriteBackStore) recover(ctx context.Context, upload fuseWriteUploadFunc) (int, error) {
	if s == nil {
		return 0, nil
	}
	entries, err := s.pending()
	if err != nil {
		return 0, err
	}
	for _, meta := range entries {
		if err := s.validateMeta(meta); err != nil {
			return 0, err
		}
		data, err := os.ReadFile(s.dataPath(meta.Path))
		if err != nil {
			return 0, fmt.Errorf("read pending write %q: %w", meta.Path, err)
		}
		if int64(len(data)) != meta.Size {
			return 0, fmt.Errorf("pending FUSE write %q size mismatch: metadata=%d actual=%d", meta.Path, meta.Size, len(data))
		}
		if _, err := upload(ctx, meta.Path, data, meta.BaseVersion, meta.BaseSize, meta.DirtyRanges); err != nil {
			return 0, fmt.Errorf("upload pending write %q: %w", meta.Path, err)
		}
		if err := s.remove(meta.Path); err != nil {
			return 0, err
		}
	}
	return len(entries), nil
}

func (s *fuseWriteBackStore) put(remotePath string, data []byte, baseVersion fuseObjectVersion, baseSize int64, dirtyRanges []fuseDirtyRange) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create FUSE write-back cache %q: %w", s.dir, err)
	}
	meta := fuseWriteBackMeta{
		Path:          remotePath,
		Size:          int64(len(data)),
		BaseSize:      baseSize,
		Mtime:         time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		CacheIdentity: s.identity,
		BaseVersion:   baseVersion,
		DirtyRanges:   append([]fuseDirtyRange(nil), dirtyRanges...),
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWriteFile(s.dataPath(remotePath), data, 0o600); err != nil {
		return fmt.Errorf("write pending FUSE data for %q: %w", remotePath, err)
	}
	if err := atomicWriteFile(s.metaPath(remotePath), append(metaData, '\n'), 0o600); err != nil {
		return fmt.Errorf("write pending FUSE metadata for %q: %w", remotePath, err)
	}
	return nil
}

func (s *fuseWriteBackStore) validateMeta(meta fuseWriteBackMeta) error {
	if meta.CacheIdentity != s.identity {
		return fmt.Errorf("pending FUSE metadata for %q belongs to a different mount identity; use the matching --cache-dir or remove %q", meta.Path, s.dir)
	}
	if meta.Size < 0 {
		return fmt.Errorf("pending FUSE metadata for %q has invalid size %d", meta.Path, meta.Size)
	}
	return nil
}

func (s *fuseWriteBackStore) remove(remotePath string) error {
	dataErr := os.Remove(s.dataPath(remotePath))
	if dataErr != nil && !os.IsNotExist(dataErr) {
		return fmt.Errorf("remove pending FUSE data for %q: %w", remotePath, dataErr)
	}
	metaErr := os.Remove(s.metaPath(remotePath))
	if metaErr != nil && !os.IsNotExist(metaErr) {
		return fmt.Errorf("remove pending FUSE metadata for %q: %w", remotePath, metaErr)
	}
	return nil
}

func (s *fuseWriteBackStore) pending() ([]fuseWriteBackMeta, error) {
	files, err := filepath.Glob(filepath.Join(s.dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	out := make([]fuseWriteBackMeta, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read pending FUSE metadata %q: %w", file, err)
		}
		var meta fuseWriteBackMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("parse pending FUSE metadata %q: %w", file, err)
		}
		if meta.Path == "" {
			return nil, fmt.Errorf("pending FUSE metadata %q is missing path", file)
		}
		out = append(out, meta)
	}
	return out, nil
}

func (s *fuseWriteBackStore) pendingStats() fuseWriteBackPendingStats {
	if s == nil {
		return fuseWriteBackPendingStats{}
	}
	entries, err := s.pending()
	if err != nil {
		return fuseWriteBackPendingStats{}
	}
	var stats fuseWriteBackPendingStats
	for _, meta := range entries {
		if err := s.validateMeta(meta); err != nil {
			continue
		}
		stats.Count++
		if meta.Size > 0 {
			stats.Bytes += meta.Size
		}
	}
	return stats
}

func (s *fuseWriteBackStore) dataPath(remotePath string) string {
	return filepath.Join(s.dir, writeBackKey(remotePath)+".dat")
}

func (s *fuseWriteBackStore) metaPath(remotePath string) string {
	return filepath.Join(s.dir, writeBackKey(remotePath)+".json")
}

func writeBackKey(remotePath string) string {
	sum := sha256.Sum256([]byte(remotePath))
	return hex.EncodeToString(sum[:])
}

func atomicWriteFile(target string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempName, mode); err != nil {
		return err
	}
	return os.Rename(tempName, target)
}
