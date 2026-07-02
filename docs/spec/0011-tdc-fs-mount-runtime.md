# tdc fs Mount Runtime

## Goal

Allow users and agents to mount and unmount tdc fs as a local filesystem.

## User-facing Commands

Initial command set:

- `tdc fs mount-file-system`
- `tdc fs unmount-file-system`

## Behavior

- Mount lifecycle stays under `tdc fs` to keep the two-level command tree.
- Commands must not prompt.
- Mount should validate local platform prerequisites before attempting to mount.
- Mount should write enough local state to support reliable unmount and status
  diagnostics.
- Unmount should be idempotent where practical and return actionable errors
  when the mount is owned by another process or cannot be found.
- Platform-specific behavior must be isolated behind build tags or small
  platform packages.

## Inputs And Config

Required inputs:

- local mount path
- resolved credentials/profile
- fs endpoint/config resolved from cloud provider and region
- stored `tdc fs` resource API key from `~/.tdc/credentials`

Optional inputs should use explicit long flags for foreground/background mode,
cache behavior, and runtime diagnostics if supported.

## Output And Errors

- JSON is the default for mount and unmount status.
- Human output may be concise status text for terminal use.
- Prerequisite failures must name the missing local dependency and the next
  action.
- Runtime state errors must identify the mount path.
- Permission errors must name `fs.mount` or the lower-level fs read/write
  permission that failed.

## After This Spec

Users can expose a `tdc fs` resource as a local filesystem:

```bash
tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace
tdc fs unmount-file-system --mount-path ./workspace
```

Agents can use ordinary local file tools against the mount after
`tdc fs mount-file-system` succeeds. The CLI still owns mount state and unmount
recovery.

## Implementation Design

- `internal/cli/fs` registers `mount-file-system` and `unmount-file-system`.
- `internal/fs/mount` owns mount orchestration and platform abstraction.
- `internal/fs/mountstate` records active mount metadata under `~/.tdc/` using
  profile, fs resource name, mount path, process id, and started time.
- `internal/fs/mountdriver` hides FUSE/WebDAV/native driver differences behind a
  small interface.
- Platform-specific files use build tags such as `_darwin.go`, `_linux.go`, and
  `_windows.go`.
- Unsupported platforms compile successfully and return a clear unsupported
  error at runtime.

## API Call Chain

Mount does not introduce a new remote control API. The runtime uses the tdc fs
data-plane client from `0010-tdc-fs-data-plane.md`:

1. Resolve profile, credentials, provider, region, and fs base URL.
2. Load the stored fs resource API key.
3. Call `GET /v1/status` with `Authorization: Bearer <api-key>` before mounting
   to verify reachability and feature capabilities.
4. Map local filesystem callbacks to data-plane calls:
   - read: `GET /v1/fs/<path>`
   - write: `PUT /v1/fs/<path>` or upload endpoints
   - list: `GET /v1/fs/<path>?list=1`
   - stat: `GET /v1/fs/<path>?stat=1`
   - remove: `DELETE /v1/fs/<path>`
5. Store local mount state under `~/.tdc/`.

No reference implementation package may be imported. Mount behavior can copy
protocol concepts only.

## Dependencies And Platform

- Prefer `github.com/hanwen/go-fuse/v2` if implementing a FUSE-backed mount
  because the reference implementation already validates its behavior.
- The mount driver must not import code from `ref/`; copy concepts only.
- FUSE support is platform-specific. Keep non-mount packages pure Go and
  cross-platform.
- Do not require cgo for the core CLI. If a future mount backend requires cgo,
  isolate it behind build tags and keep the default build cgo-free.

## Dependencies

- `0010-tdc-fs-data-plane.md`.

## Acceptance Criteria

- Unit tests cover argument validation and state file handling.
- Platform tests cover supported mount command construction.
- Smoke tests verify mount, file read/write/list, and unmount on supported
  platforms.
- Tests cover stale state cleanup and repeated unmount attempts.

## Out Of Scope

- Kernel-level filesystem implementation changes beyond what is needed to wrap
  the reference behavior.
- Mount support on unsupported operating systems.
