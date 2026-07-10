# tdc fs FUSE Correctness And Drive9 Parity Extension

## Goal

tdc MVP is intended to replace Drive9 usage on TiDB Cloud. This extension therefore has two responsibilities: improve the current tdc fs mount correctness, and record the exact Drive9 parity gap that remains after this iteration. Missing parity is not considered optional. If the current tdc code cannot safely implement a Drive9 behavior yet, this file must say what is missing and why.

## Implemented In This Extension

- FUSE cache identity is derived from profile, fs resource name, tenant id, endpoint, remote root, mount path, and tdc fs API key fingerprint.
- FUSE pending-write metadata records mount identity and base object version, and recovery rejects entries from a different mount identity.
- FUSE read cache entries store known `revision` and `resource_id` metadata and reject known-version mismatches.
- FUSE writable handles keep the object version observed at open time, retarget on rename, and are marked deleted on unlink/rmdir so a later flush does not recreate a removed file.
- FUSE upload detects best-effort stale writes by statting the current object before upload when a base version is known.
- FUSE mounts expose a Drive9-compatible local control socket in mount state, and `tdc fs drain-file-system` flushes dirty open handles plus pending write-back cache without unmounting.
- `tdc fs read-file --offset N --length M` performs byte-range reads and requires both flags together.
- Large `tdc fs copy-file --from-local --to-remote` uses the Drive9-compatible V2 multipart upload protocol first and falls back to V1 multipart upload when V2 is unavailable.
- `tdc fs copy-file --append` appends a local file to a remote file using the backend append/patch upload plan when available, with conditional rewrite fallback for compatible non-S3-backed files.
- `tdc fs copy-file --resume` resumes an active local-to-remote multipart upload and also resumes a partial remote-to-local download.
- FUSE/writeback same-size dirty writes use Drive9-style patch upload when the handle has a known base revision.
- `tdc fs copy-file --recursive` copies directory contents for local-to-remote, remote-to-local, and remote-to-remote flows.
- Local recursive copy rejects symlinks instead of silently following them.
- tdc fs layer commands and client support are implemented for `/v1/layers`, `/v1/layers/<layer_id>/*`, and `/v1/layer-checkpoints/<checkpoint_id>`.
- `tdc fs copy-file --layer-id`, `tdc fs search-file-content --layer-id`, and `tdc fs find-files --layer-id` provide Drive9-style layer-aware client workflows using tdc's explicit long flag naming.
- tdc fs-vault commands and client support are implemented for `/v1/vault/secrets`, `/v1/vault/tokens`, `/v1/vault/grants`, `/v1/vault/audit`, and `/v1/vault/read/*`.
- `tdc fs-vault run-with-secret` injects one vault secret into a child process environment, validates Drive9-style env key/value constraints, and scrubs tdc credential environment variables before exec.
- `tdc fs-vault mount-vault` exposes readable vault secrets as a read-only FUSE filesystem at `<mount>/<secret>/<field>`, and `tdc fs-vault unmount-vault` stops that mount through shared mount state.
- tdc fs-journal commands and client support are implemented for `/v1/journals`, `/v1/journals/<journal_id>/entries`, `/v1/journals/<journal_id>/verify`, and `/v1/journal-entries`.
- `tdc fs copy-file --from-stdin`, `--to-stdout`, repeatable `--tag key=value`, and `--description` are implemented for agent-friendly stream and metadata workflows.
- `tdc fs chmod-file`, `tdc fs create-symlink`, and `tdc fs create-hardlink` are implemented. tdc stores client-side POSIX metadata for mode and symlink targets under `~/.tdc/fs_metadata`, so `describe-file` and FUSE can report tdc-managed metadata even when remote stat responses omit it.
- `tdc fs pack-file-system` and `tdc fs unpack-file-system` implement a tdc-native `tdc.pack.v1` archive format and accept Drive9 `drive9.pack.v1` archives on unpack.
- Mount profiles are implemented with `coding-agent`, `portable`, and `none`; mount state records `local_root`, `mount_profile`, and `pack_paths`.
- Portable mount auto-unpack on mount and auto-pack on unmount are implemented with `--no-auto-unpack`, `--no-auto-pack`, and explicit archive path overrides.
- `tdc fs-git clone-git-workspace`, `hydrate-git-workspace`, `restore-git-workspace`, `add-git-worktree`, and `remove-git-worktree` implement Drive9-style fast Git workflows in tdc command naming.
- Low-level `tdc fs-git` workspace/tree/state/object-pack/overlay commands are implemented for diagnostics and automation.
- FUSE synthetic Git workspace handling is implemented: clean Git entries are served from `/v1/git-workspaces` tree manifests plus local `.git` objects, and dirty changes are written to Git overlay endpoints instead of ordinary `/v1/fs` rows.
- Unit tests include tdc-adapted Drive9 client-side upload cases for buffer reuse, bounded parallelism, V2 multipart part sizing, V2 presign retry, expected revision propagation, invalid tag rejection before requests, short-read abort, V1 fallback, upload resume, and append patch upload.
- Unit tests and live e2e cover the implemented range read, multipart upload, upload resume, append, download resume, recursive copy, POSIX metadata commands, layer behavior, vault behavior including mount view, journal behavior, pack/unpack behavior, and Git workspace API behavior. Unit tests also cover fast clone registration, Git restore from state/object packs, FUSE synthetic Git reads/writes, and mount auto pack/unpack.

## Implemented API Call Chain

FUSE read path:

1. `GET /v1/fs/<path>?stat=1`
2. Fallback `HEAD /v1/fs/<path>` when stat metadata is not available.
3. Check the in-memory read cache with the known object version.
4. On miss, `GET /v1/fs/<path>` and cache bytes with the known object version.

FUSE write path:

1. Keep the base object version from open/stat when available.
2. Persist pending write data and metadata locally when write-back cache is enabled.
3. Before upload, stat the current remote object when the base version is known.
4. If the remote object is missing or has a conflicting known version, fail with a stale-file error.
5. Otherwise `PUT /v1/fs/<path>`.
6. Remove the pending local entry after successful upload.

FUSE mount drain:

1. Foreground FUSE runtime creates a Unix control socket at the mountstate-derived path.
2. Mount state stores `control_socket` together with PID, driver, profile, file system name, remote root, endpoint, and read-only status.
3. `tdc fs drain-file-system --mount-path <path>` reads mount state, rejects non-FUSE mounts, rejects old mount state without a control socket, and verifies the mount process is alive.
4. The command sends a JSON `DrainRequest` with `timeout_ms` to the control socket.
5. The runtime serializes drain calls, flushes dirty open handles, recovers pending write-back cache entries, snapshots remaining pending work, and returns a Drive9-shaped `DrainResponse`.
6. Clean open handles may remain open after drain. Dirty handles or cached pending writes remaining after drain make the response non-OK.

Range read:

1. Validate `--offset` and `--length`.
2. Send `GET /v1/fs/<path>` with `Range: bytes=<offset>-<end>`.
3. If the backend returns `206 Partial Content`, stream the returned body.
4. If the backend returns `200 OK`, slice the response body locally. This preserves compatibility with inline small-file responses.
5. If the backend returns `416 Requested Range Not Satisfiable`, return an empty byte stream.

Multipart upload:

1. Prefer `POST /v2/uploads/initiate` with `path`, `total_size`, optional `expected_revision`, and optional `description`.
2. Fetch part URLs with `POST /v2/uploads/<upload_id>/presign-batch`.
3. Upload parts concurrently with bounded memory and no tdc fs Bearer auth on presigned URLs.
4. If a V2 part upload returns 403, fetch one fresh URL with `POST /v2/uploads/<upload_id>/presign` and retry that part once.
5. Complete with `POST /v2/uploads/<upload_id>/complete`, including returned ETags and optional tags.
6. On upload/complete failure, best-effort `POST /v2/uploads/<upload_id>/abort`.
7. If V2 is unavailable, fall back to V1: compute CRC32C checksums, `POST /v1/uploads/initiate`, upload returned presigned part URLs with `x-amz-checksum-crc32c`, then `POST /v1/uploads/<upload_id>/complete`.
8. On V1 upload/complete failure, best-effort `DELETE /v1/uploads/<upload_id>`.

Local-to-remote upload resume:

1. `GET /v1/uploads?path=<target>&status=UPLOADING` to find the active upload.
2. Recompute local CRC32C part checksums.
3. `POST /v1/uploads/<upload_id>/resume` with `part_checksums`.
4. If the backend is an older Drive9-compatible server that rejects the JSON body with `missing X-Dat9-Part-Checksums header`, retry `POST /v1/uploads/<upload_id>/resume` with `X-Dat9-Part-Checksums`.
5. `PUT` only the returned missing presigned part URLs.
6. `POST /v1/uploads/<upload_id>/complete`.

Append:

1. Open and stat the local append source.
2. `HEAD /v1/fs/<target>` to get the current revision, or treat 404 as a missing file with expected revision `0`.
3. For a missing target, upload the local source with expected revision `0`.
4. For an existing target, `POST /v1/fs/<target>?append` with `append_size`, `part_size`, and `expected_revision`.
5. For each returned patch upload part, optionally `GET` the returned `read_url` to read original bytes for the dirty part, concatenate the needed local append bytes, and `PUT` the presigned part URL without forwarding tdc fs Bearer auth or adding unsigned checksum headers.
6. `POST /v1/uploads/<upload_id>/complete`.
7. If the backend returns a known compatibility error such as `file is not S3-stored`, fall back to `GET /v1/fs/<target>` plus `PUT /v1/fs/<target>` with concatenated bytes and `X-Dat9-Expected-Revision: <revision>`.

FUSE dirty-range patch:

1. Track dirty byte ranges per writable FUSE handle.
2. Preserve dirty ranges and base object metadata in the write-back cache for recovery.
3. For same-size writes with a known base revision, compute dirty part numbers using the same adaptive part-size rule as append.
4. `PATCH /v1/fs/<path>` with `new_size`, `dirty_parts`, `part_size`, and `expected_revision`.
5. Upload only returned dirty parts without tdc fs Bearer auth, using `read_url` when the backend supplies original part data.
6. Complete with `POST /v1/uploads/<upload_id>/complete`.
7. Stat the remote file after patch completion to refresh the handle revision.
8. Fall back to whole-object upload for unsupported patch APIs, unknown base versions, size-changing writes, and full-file dirty writes.

POSIX metadata and stream copy:

1. `tdc fs copy-file --from-stdin --to-remote <path>` streams stdin bytes to the same write/upload path as local-to-remote copy.
2. `tdc fs copy-file --from-remote <path> --to-stdout` streams raw remote bytes to stdout and rejects `--query`.
3. Repeatable `--tag key=value` values are validated before upload and sent as `X-Dat9-Tag` headers or multipart-complete tags when available.
4. `--description` is sent as `X-Dat9-Description` on ordinary writes and multipart initiation when supported.
5. `tdc fs chmod-file` sends `POST /v1/fs/<path>?chmod` with `mode`.
6. `tdc fs create-symlink` sends `POST /v1/fs/<link>?symlink` with `target`.
7. `tdc fs create-hardlink` sends `POST /v1/fs/<link>?hardlink` with `source`.

Pack/unpack:

1. `tdc fs pack-file-system` resolves `--local-root`, `--remote-root`, mount profile, explicit `--path` values, or mount state from `--mount-path`.
2. It writes a gzip tar archive with leading `.tdc-pack-manifest.json`, format `tdc.pack.v1`, and entries under `entries/`.
3. It preserves directories, regular files, symlinks, mode bits, and mtimes, and rejects unsupported file types and unsafe paths.
4. When `--archive-path` is omitted, it writes to `/.tdc/packs/<mount-profile>-<hash>.tar.gz`.
5. The archive is uploaded as an ordinary tdc fs file with pack tags such as `tdc.pack.format=tdc.pack.v1` and `tdc.pack.profile=<profile>`.
6. `tdc fs unpack-file-system` reads the archive from tdc fs, accepts both `tdc.pack.v1` and `drive9.pack.v1`, extracts into a staging directory, validates path traversal and symlink ancestors, and then installs the staged overlay.
7. Mount auto-unpack reads the default or explicit archive before starting FUSE when pack paths exist; missing default archives are reported as a passed warning message, while missing explicit archives are errors.
8. Unmount auto-pack packs after the mount process exits and before mount state removal; if pack fails, mount state remains so the user can retry.

Git workspace workflow:

1. `tdc fs-git clone-git-workspace` resolves `--target-path` through tdc mount state, runs `git clone --no-checkout`, and optionally adds `--filter=blob:none`.
2. It reads `HEAD`, branch, and `git ls-tree -r -t -z`, then sends `POST /v1/git-workspaces` and `POST /v1/git-workspaces/<id>/tree`.
3. It runs `git read-tree --reset <head>` and applies local Git performance settings.
4. It archives lightweight `.git` state without object databases or lock files and sends `POST /v1/git-workspaces/<id>/git-state` with `storage_type=tar.gz-no-objects`.
5. `tdc fs-git hydrate-git-workspace` reads the registered tree and runs `git cat-file -e` for clean objects, causing blobless remotes to prefetch through Git.
6. `tdc fs-git add-git-worktree` uses native `git worktree add --no-checkout`, registers a linked workspace with `common_workspace_id`, replaces its tree, and checkpoints linked `.git` state.
7. `tdc fs-git remove-git-worktree` refuses dirty linked worktrees unless `--force`, deletes the linked workspace row, and removes the linked local overlay root without recursive clean-tree whiteouts.
8. FUSE loads active git workspaces, trees, and overlays from `/v1/git-workspaces`; longest root path wins.
9. FUSE reads clean files with `git cat-file -p <object_sha>` from the local overlay `.git`; overlay entries override clean tree entries, and whiteouts hide clean entries.
10. FUSE writes, chmod, symlink, hardlink copies, rename, and delete inside Git workspaces send `POST /v1/git-workspaces/<id>/overlay`.
11. `tdc fs-git restore-git-workspace --target-path <path>` downloads `GET /v1/git-workspaces/<id>/git-state`, extracts it into the local `.git` layout, lists object packs with `GET /v1/git-workspaces/<id>/object-packs`, downloads missing pack bytes with `GET /v1/git-workspaces/<id>/object-packs/<pack_id>`, and runs `git unpack-objects -r`.
12. FUSE clean Git reads call the same restore path when the workspace is registered but the local `.git` object store is missing, then retry `git cat-file`.

Remote-to-local resume:

1. Stat the local partial file.
2. `HEAD /v1/fs/<source>` to get remote size.
3. Reject when the local file is larger than the remote file.
4. If sizes differ, `GET /v1/fs/<source>` with `Range` for the missing suffix and append it locally.

Recursive copy:

1. Local-to-remote walks the local tree, rejects symlinks, creates remote directories with `POST /v1/fs/<path>?mkdir`, and uploads files with `PUT /v1/fs/<path>`.
2. Remote-to-local stats the remote root with `HEAD`, walks directories with `GET /v1/fs/<path>?list=1`, reads files with `GET`, and writes local files.
3. Remote-to-remote stats the remote root with `HEAD`, walks directories with `GET /v1/fs/<path>?list=1`, creates directories with `POST /v1/fs/<path>?mkdir`, and copies files with `POST /v1/fs/<target>?copy` plus `X-Dat9-Copy-Source`.

Layers:

1. `tdc fs create-layer` sends `POST /v1/layers` with `layer_id`, `base_root_path`, `name`, `tags`, `durability_mode`, and `actor_id`.
2. `tdc fs list-layers` sends `GET /v1/layers`.
3. `tdc fs describe-layer` sends `GET /v1/layers/<layer_id>`.
4. `tdc fs diff-layer` sends `GET /v1/layers/<layer_id>/diff` with optional `max_seq`.
5. `tdc fs replay-layer` sends `GET /v1/layers/<layer_id>/diff?replay=1` with optional `max_seq`.
6. `tdc fs create-layer-entry` sends `POST /v1/layers/<layer_id>/entries`.
7. `tdc fs upload-layer-file` sends `POST /v1/layers/<layer_id>/objects?path=<path>&size=<size>` with optional `base_revision` and `mode`.
8. `tdc fs read-layer-file` sends `GET /v1/layers/<layer_id>/objects?path=<path>` with optional `max_seq` and writes raw bytes to stdout.
9. `tdc fs describe-layer-entry` sends `GET /v1/layers/<layer_id>/entries?path=<path>` with optional `max_seq`.
10. `tdc fs create-layer-checkpoint` sends `POST /v1/layers/<layer_id>/checkpoints`.
11. `tdc fs describe-layer-checkpoint` sends `GET /v1/layer-checkpoints/<checkpoint_id>`.
12. `tdc fs list-layer-events` sends `GET /v1/layers/<layer_id>/events` with optional `since`.
13. `tdc fs rollback-layer` sends `POST /v1/layers/<layer_id>/rollback`.
14. `tdc fs commit-layer` sends `POST /v1/layers/<layer_id>/commit`, preserving the backend conflict body in the API client when the backend returns HTTP 409.
15. `tdc fs copy-file --layer-id` writes local-to-remote or remote-to-remote targets through the layer object endpoint instead of mutating `/v1/fs`.
16. `tdc fs search-file-content --layer-id` and `tdc fs find-files --layer-id` pass `layer=<layer_id>` to the backend search/find query.

Vault:

1. `tdc fs-vault create-secret` sends `POST /v1/vault/secrets` with `name`, parsed `fields`, and `created_by=tdc`.
2. `tdc fs-vault replace-secret` validates `/n/vault/<secret>`, reads one field per file from `--from-directory`, and sends `PUT /v1/vault/secrets/<name>`.
3. Owner `tdc fs-vault read-secret` sends `GET /v1/vault/secrets/<name>/value`; owner field reads send `GET /v1/vault/secrets/<name>/value/<field>` and return raw bytes when requested.
4. Delegated `tdc fs-vault read-secret --vault-token` and `TDC_VAULT_TOKEN` use `Authorization: Bearer <vault-token>` and send `GET /v1/vault/read/<name>` or `GET /v1/vault/read/<name>/<field>`.
5. `tdc fs-vault list-secrets` sends `GET /v1/vault/secrets` in owner mode and `GET /v1/vault/read` in delegated mode.
6. `tdc fs-vault delete-secret` sends `DELETE /v1/vault/secrets/<name>`.
7. `tdc fs-vault create-grant` sends `POST /v1/vault/grants` with `agent`, `scope`, `perm`, `ttl_seconds`, and optional `label_hint`; it does not send `principal_type`.
8. `tdc fs-vault delete-grant` sends `DELETE /v1/vault/grants/<grant_id>` with `revoked_by` and optional `reason`.
9. `tdc fs-vault create-token` sends `POST /v1/vault/tokens` with `agent_id`, `task_id`, `scope`, and `ttl_seconds`.
10. `tdc fs-vault delete-token` sends `DELETE /v1/vault/tokens/<token_id>`.
11. `tdc fs-vault list-audit-events` sends `GET /v1/vault/audit` with optional `secret` and capped `limit`, then applies client-side `--agent-id` and `--since` filters.
12. `tdc fs-vault run-with-secret` reads one whole secret, validates each field is an environment key matching `[A-Z_][A-Z0-9_]*`, rejects control bytes except tabs, scrubs tdc credential variables, and execs the child command.
13. `tdc fs-vault mount-vault --mount-path <path>` validates the active profile, resolves the tdc fs endpoint, probes owner or delegated vault readability, checks FUSE prerequisites, and starts a read-only background FUSE process by default.
14. The vault mount root lists readable secrets. Each secret is a directory. Each field is a read-only file whose bytes are materialized at `Open`.
15. Owner vault mount reads use `GET /v1/vault/secrets` and `GET /v1/vault/secrets/<name>/value[/<field>]`. Delegated vault mount reads use `GET /v1/vault/read` and `GET /v1/vault/read/<name>[/<field>]`.
16. A `--vault-token` value is passed to the background mount through `TDC_VAULT_TOKEN` in the child environment rather than through process arguments.
17. `tdc fs-vault unmount-vault --mount-path <path>` reuses shared mount state and process termination, with no fs auto-pack because vault mount state has no local overlay root.

Journal:

1. `tdc fs-journal create-journal` sends `POST /v1/journals` with `journal_id`, `kind`, `title`, optional `actor`, and repeated labels. When `--journal-id` is omitted, tdc generates a local `jrn_*` id.
2. `tdc fs-journal append-journal-entries` parses repeatable `--entry-json` objects, JSONL stdin, or JSON array stdin with `--json-array`, applies `--entry-type`, `--source`, and repeated `--subject`, then sends `POST /v1/journals/<journal_id>/entries`.
3. The append command sends `--idempotency-key` as `Idempotency-Key`; when omitted, tdc generates a local `app_*` id.
4. `tdc fs-journal read-journal-entries` sends `GET /v1/journals/<journal_id>/entries` with optional `after_seq` and capped `limit`, then decodes the backend NDJSON stream into structured output.
5. `tdc fs-journal search-journal-entries` sends `GET /v1/journal-entries` with Drive9-compatible query filters: `type`, `status`, `kind`, `actor`, repeated `subject`, repeated `meta`, `since`, `until`, `limit`, `cursor`, and `include=entry`.
6. `tdc fs-journal verify-journal` sends `GET /v1/journals/<journal_id>/verify`.

## Drive9 Parity Status

This iteration brings the tdc client-side filesystem surface to Drive9 parity
for the MVP replacement path on TiDB Cloud.

Aligned:

- Range read: implemented as `tdc fs read-file --offset --length`, matching Drive9 `fs cat --offset --length` behavior.
- Recursive copy: implemented for local-to-remote, remote-to-local, and remote-to-remote. Local symlinks are rejected; remote symlink traversal remains limited by the backend list/stat metadata exposed to tdc.
- Multipart upload: implemented for local-to-remote `copy-file` with V2 presign-batch upload, bounded concurrent workers, one fresh-presign retry for expired URLs, V1 fallback, internal upload summaries, and complete-time tag/description propagation in the fs client.
- Append: implemented for local-to-remote using the Drive9 append/patch upload plan when the backend supports it, with conditional rewrite fallback for non-S3-backed compatibility.
- Resume: implemented for local-to-remote active upload resume and remote-to-local partial download resume.
- FUSE patch upload: implemented for same-size dirty writes with a known base revision, including write-back cache recovery metadata.
- FUSE mount drain: implemented with a local control socket, Drive9-compatible request/response shape, dirty open-handle flush, pending write-back recovery, non-FUSE rejection, and live e2e coverage.
- FUSE correctness: improved cache isolation, stale-write checks, and open-handle rename/unlink behavior.
- Layers: implemented in tdc command style for create/list/describe/diff/replay/entry/object/checkpoint/events/rollback/commit. `copy-file --layer-id`, `search-file-content --layer-id`, and `find-files --layer-id` cover the Drive9 layer-aware client flows with explicit tdc flag names.
- Vault: implemented as top-level `tdc fs-vault` commands to preserve tdc's two-level command style. It covers owner and delegated secret reads, grants, legacy scoped tokens, audit listing, raw/env output, Drive9-style command environment injection, and a read-only mount-time vault filesystem view.
- Journal: implemented as top-level `tdc fs-journal` commands to preserve tdc's two-level command style. It covers create, append with idempotency keys, NDJSON/JSON entry input, read, search with repeated labels, and verify.
- Pack/unpack: implemented with tdc archive creation, Drive9 archive unpack compatibility, local overlay semantics, remote pack paths, and mount auto-pack/auto-unpack integration.
- POSIX metadata and stream workflows: implemented for chmod, symlink, hardlink, tags, descriptions, stdin upload, and stdout download, with client-side metadata persistence for tdc-managed mode and symlink target visibility.
- Git workspace: implemented with low-level API commands, fast clone, hydrate, restore, linked worktree add/remove, FUSE synthetic tree reads, Git overlay writes, and automatic FUSE restore when local Git state is absent.

Backend contract notes:

- tdc-created symlinks and chmod modes are materialized through client-side metadata and covered by tests. Pre-existing remote symlinks created outside tdc can only expose readlink targets if the backend exposes target metadata.
- Recursive remote copy uses backend list/stat type information for remote entries. Local recursive copy rejects local symlinks deterministically.
- No deliberate client-side gap remains in this spec for Git restore runtime or mount-time vault filesystem view.

Doctor is intentionally not in the immediate parity set for this iteration.

## Acceptance Criteria

- Unit tests cover read cache eviction, TTL, and version mismatch.
- Unit tests cover mount identity impact on default cache directory selection.
- Unit tests cover pending-write persistence, recovery, upload failure retention, and identity mismatch rejection.
- Unit tests cover open-handle retarget/delete state for rename and unlink.
- Unit tests cover Drive9-derived mount drain cases: clean open handles are allowed, dirty open handles flush, residual dirty/pending work fails, control socket request/response works, non-FUSE mounts are rejected, and old mount state without `control_socket` is rejected.
- Unit tests cover `ReadFileRange`, conditional write headers, Drive9-derived client upload cases, V2 multipart upload, V2 presign retry, V1 fallback, upload resume, append plan, append fallback, FUSE dirty-range patch upload, remote-to-local resume, recursive copy, local symlink rejection, POSIX metadata endpoints, client-side mode persistence for `describe-file`, FUSE symlink metadata readlink, stdin/stdout copy, tags/descriptions, layer API calls, layer commit conflict bodies, layer tag validation, layer object upload, layer-aware search/find query propagation, vault API calls, delegated vault reads, vault FUSE owner/delegated views, field parsing, env validation, child-process credential scrubbing, journal API calls, journal entry parsing, idempotency headers, repeated journal labels, journal search filters, pack/unpack archives, mount auto pack/unpack, fast Git clone registration, Git restore from state/object packs, FUSE synthetic Git reads/writes, and FUSE restore of missing local Git state.
- `make live-e2e` covers real tdc fs range read, V2 multipart upload, efficient append, upload resume, remote-to-local resume, recursive local-to-remote, recursive remote-to-local, recursive remote-to-remote, POSIX metadata commands, stdin/stdout copy, tags/descriptions, pack/unpack, Git workspace API CRUD, layer create/read/diff/replay/checkpoint/events/commit, vault create/read/replace/delete, delegated grant/token reads, vault audit listing, vault mount read, journal create/append/read/search/verify, FUSE mount drain, and WebDAV fallback flows.
- Backend contract notes above must be revisited when the backend exposes richer readlink/type contracts for objects created outside tdc.
