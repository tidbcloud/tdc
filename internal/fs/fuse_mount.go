//go:build !windows

package fs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	apifs "github.com/Icemap/tdc/internal/api/fs"
	"github.com/Icemap/tdc/internal/apperr"
	"github.com/Icemap/tdc/internal/fs/mountstate"
	gofs "github.com/hanwen/go-fuse/v2/fs"
	gofuse "github.com/hanwen/go-fuse/v2/fuse"
)

func (s Service) mountFUSEForeground(ctx context.Context, inputs mountInputs, remote apifs.StatusResponse, checks []MountRuntimeCheck) (MountResult, error) {
	info, err := statFuseRemote(ctx, inputs.client, inputs.remotePath)
	if err != nil {
		return MountResult{}, apperr.Wrap("fs.mount_remote_root", "runtime", 1, fmt.Sprintf("stat remote mount root %q", inputs.remotePath), err)
	}
	if !info.IsDir() {
		return MountResult{}, apperr.New("fs.mount_remote_root_not_directory", "usage", 2, fmt.Sprintf("remote mount root %q is not a directory", inputs.remotePath))
	}
	if err := os.MkdirAll(inputs.mountPath, 0o755); err != nil {
		return MountResult{}, apperr.Wrap("fs.create_mount_path", "runtime", 1, fmt.Sprintf("create mount path %q", inputs.mountPath), err)
	}

	timeout := time.Second
	root := newRemoteFuseNode(inputs.client, inputs.remotePath, inputs.readOnly)
	options := &gofs.Options{
		AttrTimeout:     &timeout,
		EntryTimeout:    &timeout,
		NegativeTimeout: &timeout,
		UID:             uint32(os.Getuid()),
		GID:             uint32(os.Getgid()),
		MountOptions: gofuse.MountOptions{
			FsName: "tdc-fs:" + inputs.fileSystemName,
			Name:   "tdcfs",
		},
	}
	if inputs.readOnly {
		options.MountOptions.Options = append(options.MountOptions.Options, "ro")
	}
	server, err := gofs.Mount(inputs.mountPath, root, options)
	if err != nil {
		return MountResult{}, apperr.Wrap("fs.mount_fuse", "runtime", 1, fmt.Sprintf("mount tdc fs with FUSE at %q", inputs.mountPath), err)
	}

	state, err := mountstate.New(inputs.profile.Name, inputs.fileSystemName, inputs.mountPath, inputs.remotePath, inputs.driver.Name(), inputs.endpoint.BaseURL, os.Getpid(), inputs.readOnly, time.Now().UTC())
	if err != nil {
		_ = server.Unmount()
		return MountResult{}, apperr.Wrap("fs.mount_state", "runtime", 1, fmt.Sprintf("create mount state for %q", inputs.mountPath), err)
	}
	stateFile, err := mountstate.Write(inputs.homeDir, state)
	if err != nil {
		_ = server.Unmount()
		return MountResult{}, apperr.Wrap("fs.write_mount_state", "runtime", 1, fmt.Sprintf("write mount state for %q", inputs.mountPath), err)
	}
	defer func() {
		_ = mountstate.Remove(inputs.homeDir, inputs.mountPath)
	}()

	signalCh := make(chan os.Signal, 2)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signalCh)
	go func() {
		select {
		case <-ctx.Done():
		case <-signalCh:
		}
		_ = server.Unmount()
	}()

	server.Wait()
	checks = append(checks, MountRuntimeCheck{Name: "mount_state", Status: "passed", Message: stateFile})
	return mountResult("unmounted", inputs, remote, checks, os.Getpid(), stateFile, ""), nil
}

type remoteFuseNode struct {
	gofs.Inode

	client     *apifs.Client
	remotePath string
	readOnly   bool
}

func newRemoteFuseNode(client *apifs.Client, remotePath string, readOnly bool) *remoteFuseNode {
	root, err := normalizeRemotePath(defaultRemotePath(remotePath))
	if err != nil {
		root = "/"
	}
	return &remoteFuseNode{client: client, remotePath: root, readOnly: readOnly}
}

var _ gofs.NodeGetattrer = (*remoteFuseNode)(nil)
var _ gofs.NodeLookuper = (*remoteFuseNode)(nil)
var _ gofs.NodeReaddirer = (*remoteFuseNode)(nil)
var _ gofs.NodeMkdirer = (*remoteFuseNode)(nil)
var _ gofs.NodeCreater = (*remoteFuseNode)(nil)
var _ gofs.NodeOpener = (*remoteFuseNode)(nil)
var _ gofs.NodeUnlinker = (*remoteFuseNode)(nil)
var _ gofs.NodeRmdirer = (*remoteFuseNode)(nil)
var _ gofs.NodeRenamer = (*remoteFuseNode)(nil)
var _ gofs.NodeSetattrer = (*remoteFuseNode)(nil)

func (n *remoteFuseNode) Getattr(ctx context.Context, f gofs.FileHandle, out *gofuse.AttrOut) syscall.Errno {
	if f != nil {
		if getter, ok := f.(gofs.FileGetattrer); ok {
			return getter.Getattr(ctx, out)
		}
	}
	info, err := statFuseRemote(ctx, n.client, n.remotePath)
	if err != nil {
		return fuseErrno(err)
	}
	fillFuseAttr(&out.Attr, info)
	return gofs.OK
}

func (n *remoteFuseNode) Lookup(ctx context.Context, name string, out *gofuse.EntryOut) (*gofs.Inode, syscall.Errno) {
	remotePath, errno := n.childPath(name)
	if errno != gofs.OK {
		return nil, errno
	}
	info, err := statFuseRemote(ctx, n.client, remotePath)
	if err != nil {
		return nil, fuseErrno(err)
	}
	fillFuseAttr(&out.Attr, info)
	child := &remoteFuseNode{client: n.client, remotePath: remotePath, readOnly: n.readOnly}
	return n.NewInode(ctx, child, fuseStableAttr(remotePath, info)), gofs.OK
}

func (n *remoteFuseNode) Readdir(ctx context.Context) (gofs.DirStream, syscall.Errno) {
	response, err := n.client.List(ctx, n.remotePath)
	if err != nil {
		return nil, fuseErrno(err)
	}
	entries := make([]gofuse.DirEntry, 0, len(response.Entries))
	for _, entry := range response.Entries {
		if entry.Name == "" || entry.Name == "." || entry.Name == ".." || strings.Contains(entry.Name, "/") {
			continue
		}
		mode := uint32(gofuse.S_IFREG)
		if entry.IsDir {
			mode = gofuse.S_IFDIR
		}
		remotePath, errno := n.childPath(entry.Name)
		if errno != gofs.OK {
			continue
		}
		entries = append(entries, gofuse.DirEntry{Name: entry.Name, Mode: mode, Ino: fuseInode(remotePath)})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return gofs.NewListDirStream(entries), gofs.OK
}

func (n *remoteFuseNode) Mkdir(ctx context.Context, name string, mode uint32, out *gofuse.EntryOut) (*gofs.Inode, syscall.Errno) {
	if n.readOnly {
		return nil, syscall.EROFS
	}
	remotePath, errno := n.childPath(name)
	if errno != gofs.OK {
		return nil, errno
	}
	perm := int64(mode & 0o777)
	if perm == 0 {
		perm = 0o755
	}
	if err := n.client.Mkdir(ctx, remotePath, perm); err != nil {
		return nil, fuseErrno(err)
	}
	info, err := statFuseRemote(ctx, n.client, remotePath)
	if err != nil {
		info = remoteFileInfo{name: path.Base(remotePath), mode: os.ModeDir | os.FileMode(perm), modTime: time.Now()}
	}
	fillFuseAttr(&out.Attr, info)
	child := &remoteFuseNode{client: n.client, remotePath: remotePath, readOnly: n.readOnly}
	return n.NewInode(ctx, child, fuseStableAttr(remotePath, info)), gofs.OK
}

func (n *remoteFuseNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *gofuse.EntryOut) (*gofs.Inode, gofs.FileHandle, uint32, syscall.Errno) {
	if n.readOnly {
		return nil, nil, 0, syscall.EROFS
	}
	remotePath, errno := n.childPath(name)
	if errno != gofs.OK {
		return nil, nil, 0, errno
	}
	if flags&uint32(syscall.O_EXCL) != 0 {
		if _, err := statFuseRemote(ctx, n.client, remotePath); err == nil {
			return nil, nil, 0, syscall.EEXIST
		} else if !errors.Is(mapWebDAVError(err), os.ErrNotExist) {
			return nil, nil, 0, fuseErrno(err)
		}
	}
	if _, err := n.client.WriteFile(ctx, remotePath, nil); err != nil {
		return nil, nil, 0, fuseErrno(err)
	}
	perm := os.FileMode(mode & 0o777)
	if perm == 0 {
		perm = 0o644
	}
	info := remoteFileInfo{name: path.Base(remotePath), mode: perm, modTime: time.Now()}
	fillFuseAttr(&out.Attr, info)
	child := &remoteFuseNode{client: n.client, remotePath: remotePath, readOnly: n.readOnly}
	handle := newRemoteFuseFile(n.client, remotePath, nil, true, false)
	return n.NewInode(ctx, child, fuseStableAttr(remotePath, info)), handle, gofuse.FOPEN_DIRECT_IO, gofs.OK
}

func (n *remoteFuseNode) Open(ctx context.Context, flags uint32) (gofs.FileHandle, uint32, syscall.Errno) {
	writable := flags&gofuse.O_ANYWRITE != 0
	if writable && n.readOnly {
		return nil, 0, syscall.EROFS
	}
	info, err := statFuseRemote(ctx, n.client, n.remotePath)
	if err != nil {
		return nil, 0, fuseErrno(err)
	}
	if info.IsDir() {
		return nil, 0, syscall.EISDIR
	}
	data := []byte{}
	if !writable || flags&uint32(syscall.O_TRUNC) == 0 {
		data, err = n.client.ReadFile(ctx, n.remotePath)
		if err != nil {
			return nil, 0, fuseErrno(err)
		}
	}
	dirty := writable && flags&uint32(syscall.O_TRUNC) != 0
	return newRemoteFuseFile(n.client, n.remotePath, data, writable, dirty), gofuse.FOPEN_DIRECT_IO, gofs.OK
}

func (n *remoteFuseNode) Unlink(ctx context.Context, name string) syscall.Errno {
	if n.readOnly {
		return syscall.EROFS
	}
	remotePath, errno := n.childPath(name)
	if errno != gofs.OK {
		return errno
	}
	if err := n.client.DeleteFile(ctx, remotePath, false); err != nil {
		return fuseErrno(err)
	}
	return gofs.OK
}

func (n *remoteFuseNode) Rmdir(ctx context.Context, name string) syscall.Errno {
	if n.readOnly {
		return syscall.EROFS
	}
	remotePath, errno := n.childPath(name)
	if errno != gofs.OK {
		return errno
	}
	if err := n.client.DeleteFile(ctx, remotePath, false); err != nil {
		return fuseErrno(err)
	}
	return gofs.OK
}

func (n *remoteFuseNode) Rename(ctx context.Context, name string, newParent gofs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	if n.readOnly {
		return syscall.EROFS
	}
	if flags != 0 {
		return syscall.ENOTSUP
	}
	source, errno := n.childPath(name)
	if errno != gofs.OK {
		return errno
	}
	parent, ok := newParent.(*remoteFuseNode)
	if !ok {
		return syscall.EIO
	}
	target, errno := parent.childPath(newName)
	if errno != gofs.OK {
		return errno
	}
	if err := n.client.Rename(ctx, source, target); err != nil {
		return fuseErrno(err)
	}
	return gofs.OK
}

func (n *remoteFuseNode) Setattr(ctx context.Context, f gofs.FileHandle, in *gofuse.SetAttrIn, out *gofuse.AttrOut) syscall.Errno {
	if f != nil {
		if setter, ok := f.(gofs.FileSetattrer); ok {
			return setter.Setattr(ctx, in, out)
		}
	}
	size, ok := in.GetSize()
	if !ok {
		return n.Getattr(ctx, f, out)
	}
	if n.readOnly {
		return syscall.EROFS
	}
	info, err := statFuseRemote(ctx, n.client, n.remotePath)
	if err != nil {
		return fuseErrno(err)
	}
	if info.IsDir() {
		return syscall.EISDIR
	}
	data, err := n.client.ReadFile(ctx, n.remotePath)
	if err != nil {
		return fuseErrno(err)
	}
	resized, errno := resizeFuseData(data, size)
	if errno != gofs.OK {
		return errno
	}
	if _, err := n.client.WriteFile(ctx, n.remotePath, resized); err != nil {
		return fuseErrno(err)
	}
	fillFuseAttrFromData(&out.Attr, n.remotePath, resized)
	return gofs.OK
}

func (n *remoteFuseNode) childPath(name string) (string, syscall.Errno) {
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") || strings.ContainsRune(name, '\x00') {
		return "", syscall.EINVAL
	}
	if n.remotePath == "/" {
		remotePath, err := normalizeRemotePath("/" + name)
		if err != nil {
			return "", syscall.EINVAL
		}
		return remotePath, gofs.OK
	}
	remotePath, err := normalizeRemotePath(path.Join(n.remotePath, name))
	if err != nil {
		return "", syscall.EINVAL
	}
	return remotePath, gofs.OK
}

type remoteFuseFile struct {
	mu         sync.Mutex
	client     *apifs.Client
	remotePath string
	data       []byte
	writable   bool
	dirty      bool
	closed     bool
	modTime    time.Time
}

func newRemoteFuseFile(client *apifs.Client, remotePath string, data []byte, writable bool, dirty bool) *remoteFuseFile {
	return &remoteFuseFile{
		client:     client,
		remotePath: remotePath,
		data:       append([]byte(nil), data...),
		writable:   writable,
		dirty:      dirty,
		modTime:    time.Now(),
	}
}

var _ gofs.FileReader = (*remoteFuseFile)(nil)
var _ gofs.FileWriter = (*remoteFuseFile)(nil)
var _ gofs.FileFlusher = (*remoteFuseFile)(nil)
var _ gofs.FileFsyncer = (*remoteFuseFile)(nil)
var _ gofs.FileReleaser = (*remoteFuseFile)(nil)
var _ gofs.FileSetattrer = (*remoteFuseFile)(nil)
var _ gofs.FileGetattrer = (*remoteFuseFile)(nil)

func (f *remoteFuseFile) Read(ctx context.Context, dest []byte, off int64) (gofuse.ReadResult, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if off < 0 {
		return nil, syscall.EINVAL
	}
	if off >= int64(len(f.data)) {
		return gofuse.ReadResultData(nil), gofs.OK
	}
	end := off + int64(len(dest))
	if end > int64(len(f.data)) {
		end = int64(len(f.data))
	}
	return gofuse.ReadResultData(append([]byte(nil), f.data[off:end]...)), gofs.OK
}

func (f *remoteFuseFile) Write(ctx context.Context, data []byte, off int64) (uint32, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.writable {
		return 0, syscall.EPERM
	}
	if off < 0 {
		return 0, syscall.EINVAL
	}
	end := off + int64(len(data))
	if end > int64(maxIntValue()) {
		return 0, syscall.EFBIG
	}
	if end > int64(len(f.data)) {
		f.data = append(f.data, bytes.Repeat([]byte{0}, int(end)-len(f.data))...)
	}
	copy(f.data[off:end], data)
	f.dirty = true
	f.modTime = time.Now()
	return uint32(len(data)), gofs.OK
}

func (f *remoteFuseFile) Flush(ctx context.Context) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.flushLocked(ctx)
}

func (f *remoteFuseFile) Fsync(ctx context.Context, flags uint32) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.flushLocked(ctx)
}

func (f *remoteFuseFile) Release(ctx context.Context) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return gofs.OK
	}
	f.closed = true
	return f.flushLocked(ctx)
}

func (f *remoteFuseFile) Setattr(ctx context.Context, in *gofuse.SetAttrIn, out *gofuse.AttrOut) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	size, ok := in.GetSize()
	if ok {
		if !f.writable {
			return syscall.EPERM
		}
		resized, errno := resizeFuseData(f.data, size)
		if errno != gofs.OK {
			return errno
		}
		f.data = resized
		f.dirty = true
		f.modTime = time.Now()
	}
	fillFuseAttrFromData(&out.Attr, f.remotePath, f.data)
	return gofs.OK
}

func (f *remoteFuseFile) Getattr(ctx context.Context, out *gofuse.AttrOut) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()
	fillFuseAttrFromData(&out.Attr, f.remotePath, f.data)
	return gofs.OK
}

func (f *remoteFuseFile) flushLocked(ctx context.Context) syscall.Errno {
	if !f.writable || !f.dirty {
		return gofs.OK
	}
	if _, err := f.client.WriteFile(ctx, f.remotePath, f.data); err != nil {
		return fuseErrno(err)
	}
	f.dirty = false
	return gofs.OK
}

func statFuseRemote(ctx context.Context, client *apifs.Client, remotePath string) (os.FileInfo, error) {
	metadata, err := client.StatMetadata(ctx, remotePath)
	if err == nil {
		return fileInfoFromMetadata(remotePath, metadata), nil
	}
	if isAPINotFound(err) {
		return nil, os.ErrNotExist
	}
	if shouldFallbackStat(err) {
		stat, statErr := client.Stat(ctx, remotePath)
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

func fillFuseAttr(attr *gofuse.Attr, info os.FileInfo) {
	mode := uint32(info.Mode().Perm())
	if info.IsDir() {
		if mode == 0 {
			mode = 0o755
		}
		attr.Mode = gofuse.S_IFDIR | mode
		attr.Size = 0
	} else {
		if mode == 0 {
			mode = 0o644
		}
		attr.Mode = gofuse.S_IFREG | mode
		if info.Size() > 0 {
			attr.Size = uint64(info.Size())
		} else {
			attr.Size = 0
		}
	}
	attr.Nlink = 1
	attr.Uid = uint32(os.Getuid())
	attr.Gid = uint32(os.Getgid())
	modTime := info.ModTime()
	attr.SetTimes(&modTime, &modTime, &modTime)
}

func fillFuseAttrFromData(attr *gofuse.Attr, remotePath string, data []byte) {
	fillFuseAttr(attr, remoteFileInfo{name: path.Base(remotePath), size: int64(len(data)), mode: 0o644, modTime: time.Now()})
}

func fuseStableAttr(remotePath string, info os.FileInfo) gofs.StableAttr {
	mode := uint32(gofuse.S_IFREG)
	if info.IsDir() {
		mode = gofuse.S_IFDIR
	}
	return gofs.StableAttr{Mode: mode, Ino: fuseInode(remotePath)}
}

func fuseInode(remotePath string) uint64 {
	var h uint64 = 1469598103934665603
	for i := 0; i < len(remotePath); i++ {
		h ^= uint64(remotePath[i])
		h *= 1099511628211
	}
	if h < 2 {
		h += 2
	}
	return h
}

func resizeFuseData(data []byte, size uint64) ([]byte, syscall.Errno) {
	if size > maxIntValue() {
		return nil, syscall.EFBIG
	}
	next := int(size)
	if next <= len(data) {
		return data[:next], gofs.OK
	}
	grown := make([]byte, next)
	copy(grown, data)
	return grown, gofs.OK
}

func maxIntValue() uint64 {
	return uint64(int(^uint(0) >> 1))
}

func fuseErrno(err error) syscall.Errno {
	if err == nil {
		return gofs.OK
	}
	err = mapWebDAVError(err)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return syscall.ENOENT
	case errors.Is(err, os.ErrPermission):
		return syscall.EACCES
	case errors.Is(err, os.ErrExist):
		return syscall.EEXIST
	case errors.Is(err, os.ErrInvalid):
		return syscall.EINVAL
	default:
		return syscall.EIO
	}
}
