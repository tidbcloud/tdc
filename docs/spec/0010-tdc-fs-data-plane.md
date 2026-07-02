# tdc fs Data Plane

## Goal

Expose filesystem operations for agents and scripts through `tdc fs`.

## User-facing Commands

Initial command set:

- `tdc fs copy-file`
- `tdc fs read-file`
- `tdc fs list-files`
- `tdc fs describe-file`
- `tdc fs move-file`
- `tdc fs delete-file`
- `tdc fs create-directory`
- `tdc fs search-file-content`
- `tdc fs find-files`

## Behavior

- Use long flags only, even when the command resembles common POSIX tools.
- Commands must not prompt.
- Preserve normal filesystem expectations where practical:
  - `read-file` writes file bytes to stdout.
  - `copy-file` transfers between local and remote paths according to explicit
    flags.
  - `list-files`, `describe-file`, `search-file-content`, and `find-files`
    provide structured results when `--output json` is used.
- Remote paths are filesystem objects and do not count as command levels.
- Mutating data-plane commands must report clear success/failure for each
  requested operation.
- Use reference filesystem behavior as guidance, but expose only tdc naming.

## Inputs And Config

- Requires resolved credentials and fs endpoint/config.
- Fs endpoint/config comes from internal provider/region resolution, not a
  user-provided server URL.
- Data-plane commands require the resource `api_key` created by
  `tdc fs create-file-system` and stored in `~/.tdc/credentials`.
- Define explicit remote path flags instead of positional-only interfaces for
  agent clarity.
- Recursive behavior, overwrite behavior, and missing-parent behavior must be
  controlled by explicit long flags.

## Output And Errors

- Commands that stream file contents may output raw bytes.
- Metadata-oriented commands should support JSON output.
- Query is valid only when the command produces structured output.
- Errors should identify the failing path and operation without leaking secret
  values.
- Permission errors must name `fs.file.read` or `fs.file.write` as appropriate.

## After This Spec

Users and agents can use `tdc fs` as a remote filesystem interface:

```bash
tdc fs create-directory --path /workspace
tdc fs copy-file --from-local ./README.md --to-remote /workspace/README.md
tdc fs list-files --path /workspace --output json
tdc fs read-file --path /workspace/README.md
tdc fs search-file-content --path /workspace --pattern "TODO"
tdc fs find-files --path /workspace --file-name-pattern "*.md"
```

This adds file operations only. Mounting remains a later runtime layer.

## Implementation Design

- `internal/cli/fs` registers data-plane subcommands beside control-plane
  subcommands.
- `internal/fs/client` defines a narrow filesystem client interface used by CLI
  commands and tests.
- `internal/fs/path` validates and normalizes remote paths.
- `internal/fs/transfer` owns upload/download/copy behavior and overwrite or
  recursive options.
- `internal/fs/search` owns grep/find request construction and response models.
- `internal/fs/fscred` loads the profile's flat `fs_api_key`.
- `internal/api/fs` implements the HTTP client behind the `internal/fs/client`
  interface.
- Raw stream commands return an `io.Reader` or stream callback to the CLI
  boundary instead of printing inside service packages.

## API Call Chain

Confirmed reference filesystem protocol endpoints. tdc may implement these
against the hosted tdc fs API after endpoint resolution is available:

- All requests use `Authorization: Bearer <tdc-fs-api-key>` after the resource
  is created.
- `GET /v1/status` to discover capabilities such as upload thresholds.
- `PUT /v1/fs/<path>` to write file bytes.
- `GET /v1/fs/<path>` to read file bytes.
- `HEAD /v1/fs/<path>` for lightweight stat metadata.
- `GET /v1/fs/<path>?stat=1` for enriched stat metadata.
- `GET /v1/fs/<path>?list=1` to list a directory.
- `DELETE /v1/fs/<path>` to remove a file or directory.
- `POST /v1/fs/<path>?create=1` to create an empty file.
- `POST /v1/fs/<path>?mkdir` with optional `mode` for directory creation.
- `POST /v1/fs/<path>?rename` with `X-Dat9-Rename-Source` for rename or move.
- `POST /v1/fs/<path>?copy` with `X-Dat9-Copy-Source` for server-side copy.
- `POST /v1/fs:batch-stat` for batch stat.
- `POST /v1/fs:batch-read-small` for batch small-file reads.
- `GET /v1/fs/<path>?grep=<query>` for grep.
- `GET /v1/fs/<path>?find=...` plus query filters for find.
- `POST /v1/uploads`, `/v1/uploads/*`, and `/v2/uploads/*` for multipart or
  large-file uploads when direct `PUT` is not appropriate.

Command mapping:

- `tdc fs read-file` uses `GET /v1/fs/<path>`.
- `tdc fs copy-file --from-local --to-remote` uses `PUT /v1/fs/<path>` for
  small files and upload endpoints for large files.
- `tdc fs copy-file --from-remote --to-local` uses `GET /v1/fs/<path>`.
- `tdc fs list-files` uses `GET /v1/fs/<path>?list=1`.
- `tdc fs describe-file` uses `GET /v1/fs/<path>?stat=1` with `HEAD` fallback
  only if the hosted API documents compatibility.
- `tdc fs delete-file` uses `DELETE /v1/fs/<path>`.
- `tdc fs create-directory` uses `POST /v1/fs/<path>?mkdir`.
- `tdc fs move-file` uses the confirmed rename endpoint/action.
- `tdc fs search-file-content` uses `GET /v1/fs/<path>?grep=<query>`.
- `tdc fs find-files` uses `GET /v1/fs/<path>?find=...`.

API gap:

- The exact mkdir, rename, copy, and large-upload request bodies must be copied
  into tdc's own protocol tests before implementation. Do not import reference
  packages.
- If the stored fs API key is missing, commands must fail with an actionable
  error suggesting `tdc fs create-file-system` or a future
  `tdc fs repair-file-system-credentials` workflow; they must not ask for a raw
  API key interactively.

## Dependencies And Platform

- No new third-party dependency is required for basic data-plane commands.
- Use standard library `io`, `os`, `path`, and `filepath` carefully: remote paths
  use slash semantics, local paths use platform-native `filepath`.
- No cgo is required.
- Platform-neutral, except tests must account for local path separator
  differences.

## Dependencies

- `0009-tdc-fs-control-plane.md`.

## Acceptance Criteria

- Tests cover each command with mocked fs client behavior.
- Tests cover raw stdout behavior for `read-file`.
- Tests cover JSON output for `list-files`, `describe-file`,
  `search-file-content`, and `find-files`.
- Tests verify data-plane requests include `Authorization: Bearer <api-key>` and
  redact the key from all error/debug paths.
- Tests cover common path errors: not found, conflict, permission denied, and
  invalid path.
- Tests verify no user-facing output exposes the reference product name.

## Out Of Scope

- Local FUSE or WebDAV mount.
- Secret vault operations.
- Git-specific workflows unless added by a later spec.
