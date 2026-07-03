package fs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/Icemap/tdc/internal/api/endpoints"
	apifs "github.com/Icemap/tdc/internal/api/fs"
	"github.com/Icemap/tdc/internal/apperr"
	"github.com/Icemap/tdc/internal/authz"
	"github.com/Icemap/tdc/internal/config"
	"github.com/Icemap/tdc/internal/dryrun"
	"github.com/Icemap/tdc/internal/fs/mountdriver"
	"github.com/Icemap/tdc/internal/fs/mountprocess"
	"github.com/Icemap/tdc/internal/fs/mountstate"
	"golang.org/x/net/webdav"
)

type MountFileSystemOptions struct {
	Profile        *config.Profile
	FileSystemName string
	MountPath      string
	RemotePath     string
	Driver         string
	Foreground     bool
	ReadOnly       bool
	ReadyTimeout   time.Duration
}

type UnmountFileSystemOptions struct {
	Profile      *config.Profile
	MountPath    string
	Timeout      time.Duration
	Force        bool
	IgnoreAbsent bool
}

type MountResult struct {
	Status         string               `json:"status"`
	Profile        string               `json:"profile"`
	FileSystemName string               `json:"file_system_name"`
	MountPath      string               `json:"mount_path"`
	RemotePath     string               `json:"remote_path"`
	Driver         string               `json:"driver"`
	PID            int                  `json:"pid,omitempty"`
	StateFile      string               `json:"state_file,omitempty"`
	LogFile        string               `json:"log_file,omitempty"`
	Endpoint       *endpoints.Endpoint  `json:"endpoint,omitempty"`
	Checks         []MountRuntimeCheck  `json:"checks,omitempty"`
	Remote         *MountRemoteSnapshot `json:"remote,omitempty"`
}

type MountRemoteSnapshot struct {
	Status   string `json:"status,omitempty"`
	TenantID string `json:"tenant_id,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Version  string `json:"version,omitempty"`
}

type UnmountResult struct {
	Status    string              `json:"status"`
	MountPath string              `json:"mount_path"`
	Driver    string              `json:"driver,omitempty"`
	PID       int                 `json:"pid,omitempty"`
	StateFile string              `json:"state_file,omitempty"`
	Checks    []MountRuntimeCheck `json:"checks,omitempty"`
}

type MountRuntimeCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type backgroundMountRequest struct {
	Executable string
	Args       []string
	LogFile    string
	StateFile  string
	MountPath  string
	Timeout    time.Duration
}

type backgroundMountStarter func(context.Context, backgroundMountRequest) (int, error)

func (s Service) MountFileSystem(ctx context.Context, opts MountFileSystemOptions) (MountResult, error) {
	inputs, err := s.mountInputs(opts)
	if err != nil {
		return MountResult{}, err
	}
	remote, err := inputs.client.Status(ctx)
	if err != nil {
		return MountResult{}, err
	}
	checks := []MountRuntimeCheck{
		{Name: "config_and_credentials", Status: "passed", Message: fmt.Sprintf("profile %q loaded", inputs.profile.Name)},
		{Name: "permission_requirement", Status: "passed", Message: string(authz.FSMount)},
		{Name: "fs_resource_credentials", Status: "passed", Message: inputs.fileSystemName},
		{Name: "endpoint_selection", Status: "passed", Message: fmt.Sprintf("%s %s", inputs.endpoint.Provider, inputs.endpoint.RegionCode)},
		{Name: "remote_status", Status: "passed", Message: statusMessage(remote.Status)},
	}
	if err := inputs.driver.CheckPrerequisites(); err != nil {
		return MountResult{}, apperr.Wrap("fs.mount_prerequisite", "runtime", 1, err.Error(), err)
	}
	checks = append(checks, MountRuntimeCheck{Name: "mount_driver", Status: "passed", Message: inputs.driver.Name()})

	if opts.Foreground {
		result, err := s.mountForeground(ctx, inputs, remote, checks)
		if err != nil {
			return MountResult{}, err
		}
		return result, nil
	}
	result, err := s.mountBackground(ctx, inputs, remote, checks)
	if err != nil {
		return MountResult{}, err
	}
	return result, nil
}

func (s Service) DryRunMountFileSystem(ctx context.Context, commandPath string, opts MountFileSystemOptions) (dryrun.Result, error) {
	inputs, err := s.mountInputs(opts)
	if err != nil {
		return dryrun.Result{}, err
	}
	checks := []dryrun.Check{
		{Name: "config_and_credentials", Status: "passed", Message: fmt.Sprintf("profile %q loaded", inputs.profile.Name)},
		{Name: "permission_requirement", Status: "passed", Message: string(authz.FSMount)},
		{Name: "fs_resource_credentials", Status: "passed", Message: inputs.fileSystemName},
		{Name: "endpoint_selection", Status: "passed", Message: fmt.Sprintf("%s %s", inputs.endpoint.Provider, inputs.endpoint.RegionCode)},
	}
	if err := inputs.driver.CheckPrerequisites(); err != nil {
		checks = append(checks, dryrun.Check{Name: "mount_driver", Status: "failed", Message: err.Error()})
	} else {
		checks = append(checks, dryrun.Check{Name: "mount_driver", Status: "passed", Message: inputs.driver.Name()})
	}
	description := "normal execution starts a local tdc fs FUSE runtime and mounts it at the requested path"
	if inputs.driver.Name() == "webdav" {
		description = "normal execution starts a local WebDAV bridge and mounts it at the requested path"
	}
	return dryrun.New(
		commandPath,
		"mount_file_system",
		dryrun.RequestSummary{
			Description: description,
			Method:      "GET",
			Path:        "/v1/status",
		},
		checks...,
	), nil
}

func (s Service) UnmountFileSystem(ctx context.Context, opts UnmountFileSystemOptions) (UnmountResult, error) {
	homeDir, err := s.homeDir()
	if err != nil {
		return UnmountResult{}, err
	}
	mountPath, err := mountstate.CanonicalMountPath(opts.MountPath)
	if err != nil {
		return UnmountResult{}, apperr.New("fs.missing_mount_path", "usage", 2, "--mount-path is required")
	}
	state, stateFile, err := mountstate.Read(homeDir, mountPath)
	if err != nil {
		if os.IsNotExist(err) && opts.IgnoreAbsent {
			return UnmountResult{
				Status:    "not_mounted",
				MountPath: mountPath,
				Checks:    []MountRuntimeCheck{{Name: "mount_state", Status: "warning", Message: "no tdc fs mount state found"}},
			}, nil
		}
		if os.IsNotExist(err) {
			return UnmountResult{}, apperr.New("fs.mount_state_not_found", "runtime", 1, fmt.Sprintf("no tdc fs mount state found for %q", mountPath))
		}
		return UnmountResult{}, apperr.Wrap("fs.read_mount_state", "runtime", 1, fmt.Sprintf("read mount state for %q", mountPath), err)
	}

	checks := []MountRuntimeCheck{{Name: "mount_state", Status: "passed", Message: stateFile}}
	if !mountprocess.Alive(state.PID) {
		if err := mountstate.Remove(homeDir, mountPath); err != nil {
			return UnmountResult{}, apperr.Wrap("fs.remove_stale_mount_state", "runtime", 1, fmt.Sprintf("remove stale mount state for %q", mountPath), err)
		}
		checks = append(checks, MountRuntimeCheck{Name: "mount_process", Status: "warning", Message: "mount process was not running; stale state removed"})
		return UnmountResult{Status: "not_mounted", MountPath: mountPath, Driver: state.Driver, PID: state.PID, StateFile: stateFile, Checks: checks}, nil
	}

	if err := mountprocess.Terminate(state.PID); err != nil {
		return UnmountResult{}, apperr.Wrap("fs.signal_mount_process", "runtime", 1, fmt.Sprintf("signal mount process %d for %q", state.PID, mountPath), err)
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if !mountprocess.WaitExit(state.PID, timeout) {
		if !opts.Force {
			return UnmountResult{}, apperr.New("fs.unmount_timeout", "runtime", 1, fmt.Sprintf("mount process %d for %q did not exit within %s; retry with --force", state.PID, mountPath, timeout))
		}
		if err := mountprocess.Kill(state.PID); err != nil {
			return UnmountResult{}, apperr.Wrap("fs.kill_mount_process", "runtime", 1, fmt.Sprintf("kill mount process %d for %q", state.PID, mountPath), err)
		}
		_ = mountprocess.WaitExit(state.PID, 5*time.Second)
	}
	_ = mountstate.Remove(homeDir, mountPath)
	checks = append(checks, MountRuntimeCheck{Name: "mount_process", Status: "passed", Message: fmt.Sprintf("process %d stopped", state.PID)})
	return UnmountResult{Status: "unmounted", MountPath: mountPath, Driver: state.Driver, PID: state.PID, StateFile: stateFile, Checks: checks}, nil
}

type mountInputs struct {
	profile        *config.Profile
	fileSystemName string
	mountPath      string
	remotePath     string
	driver         mountdriver.Driver
	endpoint       endpoints.Endpoint
	client         *apifs.Client
	readOnly       bool
	stateFile      string
	logFile        string
	homeDir        string
	timeout        time.Duration
}

func (s Service) mountInputs(opts MountFileSystemOptions) (mountInputs, error) {
	if opts.Profile == nil {
		return mountInputs{}, apperr.New("fs.missing_profile", "config", 2, "active profile is required")
	}
	fileSystemName := strings.TrimSpace(opts.FileSystemName)
	if fileSystemName == "" {
		fileSystemName = opts.Profile.FSResourceName
	}
	if fileSystemName == "" {
		return mountInputs{}, apperr.New("fs.missing_file_system_name", "usage", 2, "--file-system-name is required or fs_resource_name must exist in the active profile")
	}
	if opts.Profile.FSResourceName != "" && opts.Profile.FSResourceName != fileSystemName {
		return mountInputs{}, resourceMismatch(opts.Profile.FSResourceName, fileSystemName)
	}
	mountPath, err := mountstate.CanonicalMountPath(opts.MountPath)
	if err != nil {
		return mountInputs{}, apperr.New("fs.missing_mount_path", "usage", 2, "--mount-path is required")
	}
	remotePath, err := normalizeRemotePath(defaultRemotePath(opts.RemotePath))
	if err != nil {
		return mountInputs{}, err
	}
	driver, err := mountdriver.Resolve(strings.TrimSpace(opts.Driver))
	if err != nil {
		return mountInputs{}, apperr.New("fs.invalid_mount_driver", "usage", 2, err.Error())
	}
	endpoint, err := s.resolveFS(opts.Profile)
	if err != nil {
		return mountInputs{}, err
	}
	client, err := s.bearerClient(opts.Profile, endpoint, authz.FSMount, "mount tdc fs resource")
	if err != nil {
		return mountInputs{}, err
	}
	homeDir, err := s.homeDir()
	if err != nil {
		return mountInputs{}, err
	}
	stateFile, err := mountstate.Path(homeDir, mountPath)
	if err != nil {
		return mountInputs{}, err
	}
	logFile, err := mountstate.LogPath(homeDir, mountPath)
	if err != nil {
		return mountInputs{}, err
	}
	timeout := opts.ReadyTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return mountInputs{
		profile:        opts.Profile,
		fileSystemName: fileSystemName,
		mountPath:      mountPath,
		remotePath:     remotePath,
		driver:         driver,
		endpoint:       endpoint,
		client:         client,
		readOnly:       opts.ReadOnly,
		stateFile:      stateFile,
		logFile:        logFile,
		homeDir:        homeDir,
		timeout:        timeout,
	}, nil
}

func (s Service) mountForeground(ctx context.Context, inputs mountInputs, remote apifs.StatusResponse, checks []MountRuntimeCheck) (MountResult, error) {
	switch inputs.driver.Name() {
	case "fuse":
		return s.mountFUSEForeground(ctx, inputs, remote, checks)
	case "webdav":
		return s.mountWebDAVForeground(ctx, inputs, remote, checks)
	default:
		return MountResult{}, apperr.New("fs.invalid_mount_driver", "usage", 2, fmt.Sprintf("unsupported tdc fs mount driver %q", inputs.driver.Name()))
	}
}

func (s Service) mountWebDAVForeground(ctx context.Context, inputs mountInputs, remote apifs.StatusResponse, checks []MountRuntimeCheck) (MountResult, error) {
	prefix, err := randomWebDAVPrefix()
	if err != nil {
		return MountResult{}, apperr.Wrap("fs.mount_nonce", "runtime", 1, "create local WebDAV mount prefix", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return MountResult{}, apperr.Wrap("fs.mount_listen", "runtime", 1, "start local tdc fs WebDAV bridge", err)
	}
	defer listener.Close()

	handler := &webdav.Handler{
		Prefix:     prefix,
		FileSystem: newRemoteWebDAVFS(inputs.client, inputs.remotePath, inputs.readOnly),
		LockSystem: webdav.NewMemLS(),
	}
	server := &http.Server{Handler: handler}
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Serve(listener)
	}()

	addr := listener.Addr().(*net.TCPAddr)
	serverURL := fmt.Sprintf("http://127.0.0.1:%d%s/", addr.Port, prefix)
	if err := os.MkdirAll(inputs.mountPath, 0o755); err != nil {
		_ = server.Close()
		return MountResult{}, apperr.Wrap("fs.create_mount_path", "runtime", 1, fmt.Sprintf("create mount path %q", inputs.mountPath), err)
	}
	mountCtx, cancelMount := context.WithTimeout(ctx, inputs.timeout)
	if err := inputs.driver.Mount(mountCtx, serverURL, inputs.mountPath); err != nil {
		cancelMount()
		_ = server.Close()
		return MountResult{}, apperr.Wrap("fs.mount_driver", "runtime", 1, err.Error(), err)
	}
	cancelMount()

	state, err := mountstate.New(inputs.profile.Name, inputs.fileSystemName, inputs.mountPath, inputs.remotePath, inputs.driver.Name(), inputs.endpoint.BaseURL, os.Getpid(), inputs.readOnly, time.Now().UTC())
	if err != nil {
		_ = inputs.driver.Unmount(context.Background(), inputs.mountPath)
		_ = server.Close()
		return MountResult{}, apperr.Wrap("fs.mount_state", "runtime", 1, fmt.Sprintf("create mount state for %q", inputs.mountPath), err)
	}
	stateFile, err := mountstate.Write(inputs.homeDir, state)
	if err != nil {
		_ = inputs.driver.Unmount(context.Background(), inputs.mountPath)
		_ = server.Close()
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
		_ = inputs.driver.Unmount(context.Background(), inputs.mountPath)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err = <-serverErr
	if err != nil && err != http.ErrServerClosed {
		_ = inputs.driver.Unmount(context.Background(), inputs.mountPath)
		return MountResult{}, apperr.Wrap("fs.mount_server", "runtime", 1, "tdc fs WebDAV bridge stopped unexpectedly", err)
	}
	checks = append(checks, MountRuntimeCheck{Name: "mount_state", Status: "passed", Message: stateFile})
	return mountResult("unmounted", inputs, remote, checks, os.Getpid(), stateFile, ""), nil
}

func (s Service) mountBackground(ctx context.Context, inputs mountInputs, remote apifs.StatusResponse, checks []MountRuntimeCheck) (MountResult, error) {
	executable, err := os.Executable()
	if err != nil {
		return MountResult{}, apperr.Wrap("fs.executable_path", "runtime", 1, "determine tdc executable path for background mount", err)
	}
	if err := os.MkdirAll(filepath.Dir(inputs.logFile), 0o700); err != nil {
		return MountResult{}, apperr.Wrap("fs.mount_log_dir", "runtime", 1, fmt.Sprintf("create mount log directory %q", filepath.Dir(inputs.logFile)), err)
	}
	args := []string{
		"--profile", inputs.profile.Name,
		"fs", "mount-file-system",
		"--file-system-name", inputs.fileSystemName,
		"--mount-path", inputs.mountPath,
		"--remote-path", inputs.remotePath,
		"--driver", inputs.driver.Name(),
		"--foreground",
	}
	if inputs.readOnly {
		args = append(args, "--read-only")
	}
	pid, err := startBackgroundMount(ctx, backgroundMountRequest{
		Executable: executable,
		Args:       args,
		LogFile:    inputs.logFile,
		StateFile:  inputs.stateFile,
		MountPath:  inputs.mountPath,
		Timeout:    inputs.timeout,
	})
	if err != nil {
		return MountResult{}, err
	}
	checks = append(checks, MountRuntimeCheck{Name: "background_process", Status: "passed", Message: fmt.Sprintf("pid %d", pid)})
	checks = append(checks, MountRuntimeCheck{Name: "mount_state", Status: "passed", Message: inputs.stateFile})
	return mountResult("mounted", inputs, remote, checks, pid, inputs.stateFile, inputs.logFile), nil
}

func startBackgroundMount(ctx context.Context, request backgroundMountRequest) (int, error) {
	logFile, err := os.OpenFile(request.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, apperr.Wrap("fs.mount_log", "runtime", 1, fmt.Sprintf("open mount log %q", request.LogFile), err)
	}
	defer logFile.Close()
	cmd := exec.CommandContext(ctx, request.Executable, request.Args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		return 0, apperr.Wrap("fs.start_mount_process", "runtime", 1, "start background tdc fs mount process", err)
	}
	deadline := time.Now().Add(request.Timeout)
	for {
		if _, err := os.Stat(request.StateFile); err == nil {
			_ = cmd.Process.Release()
			return cmd.Process.Pid, nil
		}
		if !mountprocess.Alive(cmd.Process.Pid) {
			return 0, apperr.New("fs.mount_process_exited", "runtime", 1, fmt.Sprintf("background mount process exited before %q became ready; inspect %s", request.MountPath, request.LogFile))
		}
		if time.Now().After(deadline) {
			_ = mountprocess.Terminate(cmd.Process.Pid)
			return 0, apperr.New("fs.mount_ready_timeout", "runtime", 1, fmt.Sprintf("tdc fs mount at %q did not become ready within %s; inspect %s", request.MountPath, request.Timeout, request.LogFile))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func mountResult(status string, inputs mountInputs, remote apifs.StatusResponse, checks []MountRuntimeCheck, pid int, stateFile, logFile string) MountResult {
	snapshot := MountRemoteSnapshot{Status: remote.Status, TenantID: remote.TenantID, Kind: remote.Kind, Version: remote.Version}
	return MountResult{
		Status:         status,
		Profile:        inputs.profile.Name,
		FileSystemName: inputs.fileSystemName,
		MountPath:      inputs.mountPath,
		RemotePath:     inputs.remotePath,
		Driver:         inputs.driver.Name(),
		PID:            pid,
		StateFile:      stateFile,
		LogFile:        logFile,
		Endpoint:       &inputs.endpoint,
		Checks:         checks,
		Remote:         &snapshot,
	}
}

func randomWebDAVPrefix() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "/tdc-" + hex.EncodeToString(buf[:]), nil
}

func statusMessage(value string) string {
	if strings.TrimSpace(value) == "" {
		return "reachable"
	}
	return value
}

func (r MountResult) Human() string {
	lines := []string{
		"Status: " + r.Status,
		"Mount path: " + r.MountPath,
		"Remote path: " + r.RemotePath,
		"Driver: " + r.Driver,
	}
	if r.PID > 0 {
		lines = append(lines, fmt.Sprintf("PID: %d", r.PID))
	}
	if r.StateFile != "" {
		lines = append(lines, "State file: "+r.StateFile)
	}
	if r.LogFile != "" {
		lines = append(lines, "Log file: "+r.LogFile)
	}
	return strings.Join(lines, "\n")
}

func (r UnmountResult) Human() string {
	var out strings.Builder
	writer := tabwriter.NewWriter(&out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "STATUS\tMOUNT_PATH\tDRIVER\tPID")
	_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%d\n", r.Status, r.MountPath, r.Driver, r.PID)
	_ = writer.Flush()
	return strings.TrimRight(out.String(), "\n")
}
