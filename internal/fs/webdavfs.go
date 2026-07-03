package fs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/Icemap/tdc/internal/api"
	apifs "github.com/Icemap/tdc/internal/api/fs"
	"golang.org/x/net/webdav"
)

type remoteWebDAVFS struct {
	client     *apifs.Client
	remoteRoot string
	readOnly   bool
}

func newRemoteWebDAVFS(client *apifs.Client, remoteRoot string, readOnly bool) *remoteWebDAVFS {
	root, err := normalizeRemotePath(defaultRemotePath(remoteRoot))
	if err != nil {
		root = "/"
	}
	return &remoteWebDAVFS{client: client, remoteRoot: root, readOnly: readOnly}
}

func (fsys *remoteWebDAVFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	if fsys.readOnly {
		return pathErr("mkdir", name, os.ErrPermission)
	}
	remotePath, err := fsys.remotePath(name)
	if err != nil {
		return pathErr("mkdir", name, err)
	}
	if err := fsys.client.Mkdir(ctx, remotePath, int64(perm.Perm())); err != nil {
		return pathErr("mkdir", name, mapWebDAVError(err))
	}
	return nil
}

func (fsys *remoteWebDAVFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	remotePath, err := fsys.remotePath(name)
	if err != nil {
		return nil, pathErr("open", name, err)
	}
	write := flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0
	if write && fsys.readOnly {
		return nil, pathErr("open", name, os.ErrPermission)
	}

	info, statErr := fsys.stat(ctx, remotePath)
	if statErr == nil && info.IsDir() {
		entries, err := fsys.readDir(ctx, remotePath)
		if err != nil {
			return nil, pathErr("readdir", name, err)
		}
		return &remoteWebDAVFile{info: info, entries: entries}, nil
	}
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, pathErr("open", name, statErr)
	}
	if flag&os.O_EXCL != 0 && flag&os.O_CREATE != 0 && statErr == nil {
		return nil, pathErr("open", name, os.ErrExist)
	}
	if !write && statErr != nil {
		return nil, pathErr("open", name, statErr)
	}

	var data []byte
	if write && (flag&os.O_TRUNC != 0 || errors.Is(statErr, os.ErrNotExist)) {
		data = []byte{}
	} else {
		data, err = fsys.client.ReadFile(ctx, remotePath)
		if err != nil {
			return nil, pathErr("read", name, mapWebDAVError(err))
		}
	}
	if info == nil {
		info = remoteFileInfo{name: path.Base(remotePath), size: int64(len(data)), mode: 0o644, modTime: time.Now()}
	}
	offset := int64(0)
	if flag&os.O_APPEND != 0 {
		offset = int64(len(data))
	}
	return &remoteWebDAVFile{
		client:     fsys.client,
		remotePath: remotePath,
		info:       info,
		data:       data,
		offset:     offset,
		writable:   write,
	}, nil
}

func (fsys *remoteWebDAVFS) RemoveAll(ctx context.Context, name string) error {
	if fsys.readOnly {
		return pathErr("remove", name, os.ErrPermission)
	}
	remotePath, err := fsys.remotePath(name)
	if err != nil {
		return pathErr("remove", name, err)
	}
	if remotePath == fsys.remoteRoot {
		return pathErr("remove", name, os.ErrInvalid)
	}
	if err := fsys.client.DeleteFile(ctx, remotePath, true); err != nil {
		return pathErr("remove", name, mapWebDAVError(err))
	}
	return nil
}

func (fsys *remoteWebDAVFS) Rename(ctx context.Context, oldName, newName string) error {
	if fsys.readOnly {
		return pathErr("rename", oldName, os.ErrPermission)
	}
	oldRemote, err := fsys.remotePath(oldName)
	if err != nil {
		return pathErr("rename", oldName, err)
	}
	newRemote, err := fsys.remotePath(newName)
	if err != nil {
		return pathErr("rename", newName, err)
	}
	if err := fsys.client.Rename(ctx, oldRemote, newRemote); err != nil {
		return pathErr("rename", oldName, mapWebDAVError(err))
	}
	return nil
}

func (fsys *remoteWebDAVFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	remotePath, err := fsys.remotePath(name)
	if err != nil {
		return nil, pathErr("stat", name, err)
	}
	info, err := fsys.stat(ctx, remotePath)
	if err != nil {
		return nil, pathErr("stat", name, err)
	}
	return info, nil
}

func (fsys *remoteWebDAVFS) remotePath(name string) (string, error) {
	if strings.ContainsRune(name, '\x00') {
		return "", os.ErrInvalid
	}
	cleanName := path.Clean("/" + strings.TrimLeft(name, "/"))
	if cleanName == "/" {
		return fsys.remoteRoot, nil
	}
	if fsys.remoteRoot == "/" {
		return normalizeRemotePath(cleanName)
	}
	return normalizeRemotePath(path.Join(fsys.remoteRoot, strings.TrimLeft(cleanName, "/")))
}

func (fsys *remoteWebDAVFS) stat(ctx context.Context, remotePath string) (os.FileInfo, error) {
	metadata, err := fsys.client.StatMetadata(ctx, remotePath)
	if err == nil {
		return fileInfoFromMetadata(remotePath, metadata), nil
	}
	if isAPINotFound(err) {
		return nil, os.ErrNotExist
	}
	if shouldFallbackStat(err) {
		stat, statErr := fsys.client.Stat(ctx, remotePath)
		if statErr == nil {
			return fileInfoFromStat(remotePath, stat), nil
		}
		if isAPINotFound(statErr) {
			return nil, os.ErrNotExist
		}
		return nil, mapWebDAVError(statErr)
	}
	return nil, mapWebDAVError(err)
}

func (fsys *remoteWebDAVFS) readDir(ctx context.Context, remotePath string) ([]os.FileInfo, error) {
	response, err := fsys.client.List(ctx, remotePath)
	if err != nil {
		return nil, mapWebDAVError(err)
	}
	entries := make([]os.FileInfo, 0, len(response.Entries))
	for _, entry := range response.Entries {
		mode := os.FileMode(0o644)
		if entry.IsDir {
			mode = os.ModeDir | 0o755
		}
		entries = append(entries, remoteFileInfo{
			name:    entry.Name,
			size:    entry.Size,
			mode:    mode,
			modTime: unixModTime(entry.Mtime),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	return entries, nil
}

type remoteWebDAVFile struct {
	client     *apifs.Client
	remotePath string
	info       os.FileInfo
	entries    []os.FileInfo
	data       []byte
	offset     int64
	writable   bool
	closed     bool
}

func (f *remoteWebDAVFile) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true
	if !f.writable {
		return nil
	}
	if _, err := f.client.WriteFile(context.Background(), f.remotePath, f.data); err != nil {
		return mapWebDAVError(err)
	}
	return nil
}

func (f *remoteWebDAVFile) Read(p []byte) (int, error) {
	if f.info != nil && f.info.IsDir() {
		return 0, os.ErrInvalid
	}
	if f.offset >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.offset:])
	f.offset += int64(n)
	return n, nil
}

func (f *remoteWebDAVFile) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = f.offset + offset
	case io.SeekEnd:
		next = int64(len(f.data)) + offset
	default:
		return 0, os.ErrInvalid
	}
	if next < 0 {
		return 0, os.ErrInvalid
	}
	f.offset = next
	return f.offset, nil
}

func (f *remoteWebDAVFile) Readdir(count int) ([]os.FileInfo, error) {
	if f.info == nil || !f.info.IsDir() {
		return nil, os.ErrInvalid
	}
	if f.offset >= int64(len(f.entries)) && count > 0 {
		return nil, io.EOF
	}
	start := int(f.offset)
	end := len(f.entries)
	if count > 0 && start+count < end {
		end = start + count
	}
	out := append([]os.FileInfo(nil), f.entries[start:end]...)
	f.offset = int64(end)
	return out, nil
}

func (f *remoteWebDAVFile) Stat() (os.FileInfo, error) {
	if f.info != nil {
		return f.info, nil
	}
	return remoteFileInfo{name: path.Base(f.remotePath), size: int64(len(f.data)), mode: 0o644, modTime: time.Now()}, nil
}

func (f *remoteWebDAVFile) Write(p []byte) (int, error) {
	if !f.writable {
		return 0, os.ErrPermission
	}
	if f.offset < 0 {
		return 0, os.ErrInvalid
	}
	end := f.offset + int64(len(p))
	if end > int64(len(f.data)) {
		grown := bytes.Repeat([]byte{0}, int(end)-len(f.data))
		f.data = append(f.data, grown...)
	}
	copy(f.data[f.offset:end], p)
	f.offset = end
	f.info = remoteFileInfo{name: path.Base(f.remotePath), size: int64(len(f.data)), mode: 0o644, modTime: time.Now()}
	return len(p), nil
}

type remoteFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

func (i remoteFileInfo) Name() string {
	if i.name == "" || i.name == "." || i.name == "/" {
		return "/"
	}
	return path.Base(i.name)
}

func (i remoteFileInfo) Size() int64 {
	return i.size
}

func (i remoteFileInfo) Mode() os.FileMode {
	return i.mode
}

func (i remoteFileInfo) ModTime() time.Time {
	return i.modTime
}

func (i remoteFileInfo) IsDir() bool {
	return i.mode.IsDir()
}

func (i remoteFileInfo) Sys() any {
	return nil
}

func fileInfoFromMetadata(remotePath string, metadata apifs.StatMetadataResponse) os.FileInfo {
	mode := os.FileMode(0o644)
	if metadata.IsDir {
		mode = os.ModeDir | 0o755
	}
	return remoteFileInfo{name: path.Base(remotePath), size: metadata.Size, mode: mode, modTime: unixModTime(metadata.Mtime)}
}

func fileInfoFromStat(remotePath string, stat apifs.StatResponse) os.FileInfo {
	mode := os.FileMode(0o644)
	if stat.HasMode {
		mode = os.FileMode(stat.Mode)
	}
	if stat.IsDir {
		mode |= os.ModeDir
		if stat.Mode == 0 {
			mode |= 0o755
		}
	}
	return remoteFileInfo{name: path.Base(remotePath), size: stat.SizeBytes, mode: mode, modTime: unixModTime(stat.Mtime)}
}

func unixModTime(value int64) time.Time {
	if value <= 0 {
		return time.Now()
	}
	return time.Unix(value, 0)
}

func mapWebDAVError(err error) error {
	if err == nil {
		return nil
	}
	if isAPINotFound(err) {
		return os.ErrNotExist
	}
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusForbidden, http.StatusUnauthorized:
			return os.ErrPermission
		case http.StatusConflict:
			return os.ErrExist
		}
	}
	return err
}

func isAPINotFound(err error) bool {
	var apiErr *api.Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func pathErr(op, name string, err error) error {
	if err == nil {
		return nil
	}
	return &os.PathError{Op: op, Path: name, Err: err}
}
