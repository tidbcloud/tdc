# Drive9 Companion Wrapper For tdc fs

> **Current status:** The unconditional Drive9 wrapper ownership boundary is implemented and remains current. The shared companion HOME and flat `fs_*` storage below are superseded by `0016-profile-fs-resource-registry.md`, which uses one isolated companion HOME and credentials directory per profile/resource. `0018-fs-token-auth-and-config-free-access.md` adds non-persistent FS-token access. The current installer and updater download the platform Drive9 artifact plus checksum from the unversioned Drive9 release endpoint; the pinned-version range, companion version negotiation, and structured compatibility reporting proposed below are not implemented and must not be presented as current behavior. There is still no native tdc FS fallback.

## Goal

Replace the native tdc implementation of `tdc fs`, `tdc fs-git`, `tdc fs-journal`, and `tdc fs-vault` with a wrapper around a pinned compatible Drive9 companion binary. tdc remains the TiDB Cloud Starter CLI and keeps the user-facing `tdc` command surface, while Drive9 owns filesystem data-plane behavior, FUSE/WebDAV mount runtime correctness, layers, journal, vault, git workspace, pack, and unpack semantics. There must be no hidden runtime fallback to the old tdc-native fs client.

This spec supersedes the long-term native-client direction from `0010-tdc-fs-data-plane.md`, `0011-tdc-fs-mount-runtime.md`, `0011-ext01-fuse-cache-and-open-handle-correctness.md`, and `0014-tdc-fs-unix-command-aliases.md` for filesystem client behavior. Those completed specs remain useful as command-surface and test-history context, but the implementation target becomes wrapper parity with Drive9 instead of reimplementing Drive9 inside tdc.

## Rationale

Drive9 already has the mature client-side filesystem implementation that tdc needs: upload resume, append behavior, recursive copy, FUSE and WebDAV mount runtimes, mount drain, layer operations, vault views, journal operations, git workspace workflows, pack/unpack, metadata commands, and edge-case tests around cache and open-handle correctness. Reimplementing those semantics in tdc creates a persistent parity and data-safety risk. The recent FUSE overwrite bug is the concrete failure mode this spec is meant to remove.

tdc should integrate that mature runtime as an external companion binary rather than importing packages from `ref/drive9`. The `ref/` directory remains reference-only. Main tdc code must not import, replace, test-link, or build against anything under `ref/`.

## User-facing Commands

Keep the tdc command surface. Users do not call Drive9 directly for normal tdc workflows:

```bash
tdc fs create-file-system
tdc fs delete-file-system
tdc fs check-file-system
tdc fs copy-file
tdc fs read-file
tdc fs list-files
tdc fs describe-file
tdc fs move-file
tdc fs delete-file
tdc fs create-directory
tdc fs chmod-file
tdc fs create-symlink
tdc fs create-hardlink
tdc fs search-file-content
tdc fs find-files
tdc fs mount-file-system
tdc fs drain-file-system
tdc fs unmount-file-system
tdc fs pack-file-system
tdc fs unpack-file-system
tdc fs create-layer
tdc fs list-layers
tdc fs describe-layer
tdc fs diff-layer
tdc fs create-layer-checkpoint
tdc fs rollback-layer
tdc fs commit-layer
tdc fs-git clone-git-workspace
tdc fs-git hydrate-git-workspace
tdc fs-git add-git-worktree
tdc fs-git remove-git-worktree
tdc fs-journal create-journal
tdc fs-journal append-journal-entries
tdc fs-journal read-journal-entries
tdc fs-journal search-journal-entries
tdc fs-journal verify-journal
tdc fs-vault create-secret
tdc fs-vault replace-secret
tdc fs-vault read-secret
tdc fs-vault list-secrets
tdc fs-vault delete-secret
tdc fs-vault create-grant
tdc fs-vault delete-grant
tdc fs-vault list-audit-events
tdc fs-vault run-with-secret
tdc fs-vault mount-vault
tdc fs-vault unmount-vault
```

Unix-style aliases from `0014` remain aliases only. Flags stay long-form on the tdc side.

Initial command mapping:

| tdc command | Drive9 companion command |
| --- | --- |
| `tdc fs copy-file`, `tdc fs cp` | `tdc-drive9 fs cp` |
| `tdc fs read-file`, `tdc fs cat` | `tdc-drive9 fs cat` |
| `tdc fs list-files`, `tdc fs ls` | `tdc-drive9 fs ls` |
| `tdc fs describe-file`, `tdc fs stat` | `tdc-drive9 fs stat` |
| `tdc fs move-file`, `tdc fs mv` | `tdc-drive9 fs mv` |
| `tdc fs delete-file`, `tdc fs rm` | `tdc-drive9 fs rm` |
| `tdc fs create-directory`, `tdc fs mkdir` | `tdc-drive9 fs mkdir` |
| `tdc fs chmod-file`, `tdc fs chmod` | `tdc-drive9 fs chmod` |
| `tdc fs create-symlink`, `tdc fs symlink` | `tdc-drive9 fs symlink` |
| `tdc fs create-hardlink`, `tdc fs hardlink` | `tdc-drive9 fs hardlink` |
| `tdc fs search-file-content`, `tdc fs grep` | `tdc-drive9 fs grep` |
| `tdc fs find-files`, `tdc fs find` | `tdc-drive9 fs find` |
| `tdc fs mount-file-system`, `tdc fs mount` | `tdc-drive9 mount` |
| `tdc fs drain-file-system`, `tdc fs drain` | `tdc-drive9 mount drain` |
| `tdc fs unmount-file-system`, `tdc fs umount` | `tdc-drive9 umount` |
| `tdc fs create-layer` | `tdc-drive9 fs layer create` |
| `tdc fs list-layers` | `tdc-drive9 fs layer list` |
| `tdc fs describe-layer` | `tdc-drive9 fs layer status` |
| `tdc fs diff-layer` | `tdc-drive9 fs layer diff` |
| `tdc fs create-layer-checkpoint` | `tdc-drive9 fs layer checkpoint` |
| `tdc fs rollback-layer` | `tdc-drive9 fs layer rollback` |
| `tdc fs commit-layer` | `tdc-drive9 fs layer commit` |
| `tdc fs pack-file-system` | `tdc-drive9 pack` |
| `tdc fs unpack-file-system` | `tdc-drive9 unpack` |
| `tdc fs-git clone-git-workspace` | `tdc-drive9 git clone` |
| `tdc fs-git hydrate-git-workspace` | `tdc-drive9 git hydrate` |
| `tdc fs-git add-git-worktree` | `tdc-drive9 git worktree add` |
| `tdc fs-git remove-git-worktree` | `tdc-drive9 git worktree remove` |
| `tdc fs-journal ...` | `tdc-drive9 journal ...` |
| `tdc fs-vault create/read/list/delete/replace secret` | `tdc-drive9 vault set/get/ls/rm/put` |
| `tdc fs-vault create-grant/delete-grant/list-audit-events/run-with-secret` | `tdc-drive9 vault grant/revoke/audit/with` |
| `tdc fs-vault mount-vault`, `tdc fs-vault unmount-vault` | `tdc-drive9 mount vault`, `tdc-drive9 umount` |

The tdc command list must match Drive9's public external CLI surface, not Drive9 internal backend APIs or SDK-only helpers. Do not keep or test tdc commands for operations that Drive9 does not expose as CLI commands. Explicitly excluded from tdc are low-level layer entry/object/event commands, low-level Git workspace/tree/state/object-pack/overlay commands, Git restore runtime commands that are not exposed by Drive9 CLI, and legacy vault token commands. If Drive9 later exposes one of those operations publicly, add it through a new spec, README update, AGENTS update, and e2e coverage.

## Behavior

- tdc installs and invokes a pinned Drive9 companion binary. The binary is the Drive9 CLI runtime, but tdc installs it under a tdc-managed name such as `tdc-drive9` by default so it does not shadow or mutate a user's standalone `drive9`.
- tdc keeps `~/.tdc/` as the user-facing source of truth. Users must not be asked to edit `~/.drive9`.
- The wrapper translates tdc profile, region, filesystem resource, and credential state into the environment and flags expected by Drive9.
- If the companion is missing or incompatible, fs commands fail with an actionable tdc error. They must not silently fall back to a tdc-owned HTTP/FUSE/WebDAV implementation.
- `tdc fs create-file-system` provisions the resource through the companion and persists the returned tdc fs resource metadata and API key back into the active tdc profile. Optional `--wait` invokes the public Drive9 `fs stat --output json :/` command until the root becomes readable or the ten-minute deadline expires.
- `tdc fs delete-file-system` submits deletion for only the selected tdc-managed resource and removes that resource's local registry metadata and API key after Drive9 accepts the request. Because remote deletion is asynchronous, the structured result reports `status: "deleting"` and `remote_deletion_state: "deleting"`, not `deleted`.
- A failed readiness wait never deletes the provisioned resource or removes its local credentials. The error identifies the file system and states that it remains registered for inspection or retry.
- `tdc fs check-file-system` verifies that the active profile has fs metadata and that the Drive9 companion can reach the resource.
- Data-plane and mount commands stream through Drive9. tdc must not buffer arbitrary file contents just to normalize output.
- `tdc fs create-directory --mode` remains accepted for tdc CLI compatibility and must validate the octal value, but Drive9's public `mkdir` command does not currently apply directory modes. Do not emulate directory chmod through a non-public backend call.
- FUSE remains the default mount runtime where the platform and Drive9 support it. WebDAV is used only when explicitly requested or when Drive9 reports a supported fallback path.
- FUSE-only mount flags such as read-cache size, read-cache max-file size, read-cache TTL, cache directory, and writeback durability must be passed to Drive9 only when the user explicitly selects `--driver fuse`; they must not be sent for `--driver auto` because Drive9 may choose WebDAV.
- `tdc fs drain-file-system` routes to Drive9 mount drain only for actual FUSE mounts. If Drive9 selects or the user requests WebDAV, writes flush through normal file close semantics and tdc must not treat drain as a required WebDAV capability.
- `tdc fs-vault mount-vault` requires a delegated vault token minted by `tdc fs-vault create-grant`. Owner `fs_api_key` still covers owner vault management/read commands, but the Drive9 vault mount consumption path binds to `DRIVE9_VAULT_TOKEN`.
- Missing fs configuration must fail before invoking Drive9 with an actionable tdc error, for example: `tdc [ERROR]: authentication required: missing fs_api_key for profile "default"; run tdc fs create-file-system first`.
- Missing or incompatible companion binary must fail with an actionable error that tells the user to run the tdc installer or `tdc update`.
- Debug logs must show command names and sanitized argument shapes but never show TiDB Cloud keys, `fs_api_key`, vault tokens, SQL credentials, file contents, or secret values.

## Inputs And Config

tdc continues to store fs state in `~/.tdc/config` and `~/.tdc/credentials` under the active profile:

```toml
# ~/.tdc/config
[default]
region_code = "aws-us-east-1"
fs_resource_name = "workspace"
fs_tenant_id = "tenant-..."
fs_cloud_provider = "aws"
fs_region_code = "aws-us-east-1"

# ~/.tdc/credentials
[default]
tdc_public_key = "..."
tdc_private_key = "..."
fs_api_key = "..."
```

The wrapper creates an isolated Drive9 home under `~/.tdc`, for example:

```text
~/.tdc/drive9-home/.drive9/config
~/.tdc/drive9-home/.cache/drive9/
```

When invoking the companion, tdc sets:

- `HOME=~/.tdc/drive9-home` so Drive9 runtime state stays under `~/.tdc/drive9-home`.
- `DRIVE9_API_KEY=<fs_api_key>` for fs data-plane and owner-resource commands.
- `DRIVE9_SERVER=<resolved fs server>` from the tdc fs region manifest resolver.
- `DRIVE9_REGION_CODE=<tdc canonical region code>` for provisioning paths.
- `DRIVE9_PUBLIC_KEY=<tdc_public_key>` and `DRIVE9_PRIVATE_KEY=<tdc_private_key>` only for provisioning and deletion paths that require TiDB Cloud keys.

The child process environment must be built from an allowlist plus required inherited basics such as `PATH`, `HOME` replacement, `TMPDIR`, locale variables, and platform variables needed for FUSE. User-provided `DRIVE9_API_KEY`, `DRIVE9_SERVER`, `DRIVE9_PUBLIC_KEY`, `DRIVE9_PRIVATE_KEY`, and `DRIVE9_REGION_CODE` must not override tdc profile values. Profiling or debug-only Drive9 variables may be passed through only after an explicit decision and tests.

`tdc fs create-file-system` should call the companion with explicit non-interactive arguments equivalent to:

```bash
tdc-drive9 create \
  --json \
  --name <file-system-name> \
  --region-code <tdc-region-code> \
  --tidbcloud-public-key <redacted> \
  --tidbcloud-private-key <redacted>
```

The wrapper parses the JSON result, validates that it contains the resource API key, server, tenant/resource identity, and region, then writes tdc profile state. If Drive9's create output lacks a field tdc needs, tdc must fail without partially writing credentials.

## Installation And Update

The tdc installer must install both:

- `tdc`
- a compatible Drive9 companion binary, installed as `tdc-drive9` or another tdc-owned executable name

The installer must not pipe `https://drive9.ai/install.sh` as-is because that installer installs `drive9` and bootstraps the user's real `~/.drive9/config`. tdc needs an isolated companion installation and tdc-owned state under `~/.tdc`.

Installer requirements:

- Detect OS and architecture for both binaries.
- Download a pinned compatible Drive9 companion artifact with checksum verification.
- Install the companion next to `tdc` when possible, or store the absolute companion path in tdc install metadata.
- Upgrade an existing tdc-managed companion binary during tdc install/update.
- Preserve normal `tdc` PATH shadowing checks.
- Avoid changing or deleting a user-installed standalone `drive9`.
- Print clear next steps that mention both TiDB Cloud DB commands and tdc fs commands.
- Print supported tdc DB regions and tdc fs regions after installation.

`tdc update` must handle companion compatibility:

- `tdc update --check` reports the tdc version, companion version, required companion range, and compatibility status.
- `tdc update` upgrades both tdc and the companion when the direct installer channel owns the installation.
- Package-manager installs from a future Homebrew/Scoop spec must refuse in-place replacement and tell the user to use the package manager.
- If the companion is missing or incompatible, fs commands fail with `update.required` or a similarly stable actionable error instead of running an unknown binary.

`tdc --version` should include companion status in text output or expose it through a structured version command:

```json
{
  "tdc_version": "v0.1.0",
  "drive9_companion_version": "vX.Y.Z",
  "drive9_companion_compatible": true
}
```

## Output And Errors

The wrapper has two output modes:

- Pass-through mode for raw data, streaming file content, mount lifecycle, pack/unpack, and commands whose Drive9 output is already the user-facing interface.
- Captured structured mode for tdc commands that need JSON parsing, state persistence, `--query`, or tdc error categorization.

Rules:

- `tdc fs read-file` streams bytes by default and rejects `--query`.
- Commands that can safely request Drive9 JSON should translate `--output json` into Drive9's JSON flag and apply tdc `--query` after parsing.
- `--output text` may render Drive9 text output directly when the Drive9 output is already stable and human-readable.
- Drive9 errors may be passed through for low-level fs and mount failures, but tdc-owned preflight errors must use the normal `tdc [ERROR]: ...` prefix and stable exit-code categories.
- Secrets must be redacted in all captured errors, debug logs, telemetry, tests, and docs.

## Implementation Design

Add a wrapper package, for example `internal/fswrap`, with these responsibilities:

- Locate the companion binary from install metadata, a configured absolute path for tests, or a known executable next to `tdc`.
- Check companion version and compatibility.
- Resolve the active tdc profile and fs resource credentials.
- Resolve the tdc fs server from profile region and the hosted region manifest.
- Build sanitized Drive9 environment variables.
- Translate tdc command flags and paths into Drive9 arguments.
- Execute the companion with stdin/stdout/stderr passthrough for streaming commands.
- Capture stdout/stderr only for commands that need JSON parsing or tdc state updates.
- Redact secrets from returned errors and debug output.

`internal/cli` should keep command registration and validation in tdc. Command handlers call the Drive9 wrapper path instead of native tdc HTTP client and mount runtime code.

`internal/fs` may keep tdc-owned option/result types and thin service methods so the rest of the CLI does not need to know Drive9 details. Those service methods must route unconditionally to the wrapper implementation. Do not keep a production switch such as `UseDrive9Companion`, and do not keep a fallback path that executes tdc-native data-plane, FUSE, WebDAV, layer, journal, vault, git, pack, or upload logic when Drive9 is unavailable.

Old native tests that asserted tdc's own HTTP/FUSE/WebDAV behavior should be removed or rewritten as wrapper tests. tdc unit tests should use a fake companion binary to verify argument translation, environment sanitization, profile writes, profile cleanup, output decoding, and missing-companion errors. Deep filesystem correctness tests belong to Drive9.

The implementation must not import packages from `ref/drive9`, must not add `replace` entries pointing into `ref/`, and must not make tests depend on reference code or fixtures under `ref/`.

## API Call Chain

Provisioning:

1. `tdc fs create-file-system` loads the active tdc profile and TiDB Cloud API keys.
2. tdc resolves the canonical fs region and server from profile region.
3. tdc invokes `tdc-drive9 create --json ...` with a sanitized environment.
4. Drive9 calls its `/v1/provision` backend with TiDB Cloud keys and region parameters.
5. Drive9 returns resource metadata and a tdc fs data-plane API key.
6. tdc writes non-secret fs metadata to `~/.tdc/config` and `fs_api_key` to `~/.tdc/credentials`.
7. When `--wait` is set, tdc invokes `tdc-drive9 fs stat --output json :/` with the stored resource credentials every five seconds until it succeeds or ten minutes elapse.

Data-plane command:

1. `tdc fs copy-file` loads the active tdc profile.
2. tdc resolves `fs_api_key` and fs server.
3. tdc invokes `tdc-drive9 fs cp ...` with `DRIVE9_API_KEY` and `DRIVE9_SERVER`.
4. Drive9 performs the data-plane API calls and streams output/errors back through tdc.

Mount command:

1. `tdc fs mount-file-system` validates fs profile state and companion compatibility.
2. tdc invokes `tdc-drive9 mount ...` with the translated remote path, mount path, and runtime flags.
3. Drive9 owns FUSE/WebDAV negotiation, daemonization, cache, writeback, open-handle correctness, drain, and unmount behavior.

## Dependencies And Platform

- Runtime dependency: pinned compatible Drive9 companion binary.
- tdc itself does not gain a cgo requirement from this spec.
- Drive9 companion platform support determines fs feature availability.
- FUSE prerequisites remain platform-specific. tdc should surface Drive9's doctor or prerequisite messages where possible.
- macOS and Linux should use Drive9 FUSE by default when available.
- WebDAV remains available through Drive9 for unsupported or explicitly requested environments.
- Windows support depends on Drive9's supported WebDAV/runtime path.

## Testing

Unit and e2e coverage must focus on tdc wrapper behavior:

- Companion binary discovery and missing-binary errors.
- Version compatibility checks.
- Environment sanitization and secret redaction.
- tdc profile to Drive9 environment mapping.
- Argument translation for every fs, fs-git, fs-journal, and fs-vault command.
- `create-file-system` JSON parsing and all-or-nothing tdc config writes.
- `create-file-system --wait` success, transient retry, timeout,
  cancellation, and preservation of locally stored credentials on failure.
- `delete-file-system` cleanup behavior and asynchronous `deleting` output.
- Raw streaming behavior for `read-file`.
- `--query` rejection on raw commands and `--query` support on captured JSON commands.
- No tests should be retained for tdc commands that are not part of the Drive9 public CLI surface. Low-level layer entry/object/event commands, low-level Git workspace/tree/state/object-pack/overlay commands, and legacy vault token commands should not appear in command-surface, e2e, or README examples.
- No unit tests should assert tdc-native HTTP data-plane, FUSE, WebDAV, vault mount, pack, journal, layer, or Git runtime internals as product behavior. If such helpers remain temporarily in the tree, they are not the tdc fs contract and must not be reachable from CLI service methods.

Live e2e must run real commands through the wrapper:

- create a temporary tdc fs resource with `--wait`
- check it
- copy/read/list/describe/move/delete files
- create directories and recursive copies
- chmod, symlink, hardlink, tags/description if supported by Drive9
- search and find
- pack and unpack
- mount through FUSE where the runner supports FUSE
- run mount drain
- verify the overwrite-existing-file regression does not truncate data
- verify data-plane writes are visible through the mount and mount writes are visible through data-plane commands
- run representative fs-git, fs-journal, and fs-vault workflows
- delete only the resource created by that test run

Drive9 owns deep FUSE correctness tests. tdc must still carry regression tests that prove tdc's wrapper path uses Drive9 and does not reintroduce data loss through translation, environment, or lifecycle mistakes.

## After This Spec

Users still use tdc commands:

```bash
tdc fs create-file-system --file-system-name workspace --wait
tdc fs cp --from-local ./README.md --to-remote /README.md
tdc fs cat --path /README.md
tdc fs mount-file-system --mount-path ./mnt --remote-path /
tdc fs drain-file-system --mount-path ./mnt
tdc fs unmount-file-system --mount-path ./mnt
tdc fs-git add-git-worktree --base-path ./repo --worktree-path ./repo-feature --branch-name feature-x
tdc fs-journal append-journal-entries --journal-id demo --entry-json '{"type":"deployed"}'
tdc fs-vault create-secret --secret-name database --field DATABASE_URL=mysql://example
tdc fs-vault create-grant --agent-id demo-agent --scope database --permission read --ttl 10m
tdc fs-vault mount-vault --mount-path ./vault --vault-token "$TDC_VAULT_TOKEN"
```

The user-facing experience remains tdc, but filesystem correctness and advanced fs-side capabilities come from the Drive9 companion runtime.

## Acceptance Criteria

- tdc installer installs `tdc` and a compatible Drive9 companion binary with checksum verification.
- `tdc update --check` reports companion compatibility.
- `tdc update` updates the companion for direct-installer installs.
- `tdc fs create-file-system` provisions through the companion and writes only tdc-owned state under `~/.tdc`.
- `tdc fs create-file-system --wait` returns `status: "ready"` only after the public Drive9 root stat succeeds; failures retain the resource and credentials.
- `tdc fs delete-file-system` reports asynchronous remote state as `deleting`.
- `tdc fs check-file-system` succeeds for a configured profile without requiring user-visible `~/.drive9` setup.
- All retained public `tdc fs` and Unix-alias commands route through the companion.
- `tdc fs mount-file-system` defaults to Drive9 FUSE where available and supports explicit WebDAV when Drive9 supports it.
- `tdc fs drain-file-system` routes to Drive9 mount drain for FUSE mounts and reports a clear unsupported runtime error when used against WebDAV.
- `tdc fs-git`, `tdc fs-journal`, and `tdc fs-vault` route to Drive9 and expose the Drive9-backed workflows under tdc naming, including delegated-token vault mount.
- Pack/unpack and layer commands are available through tdc.
- Missing companion binary, incompatible companion version, missing fs profile state, unsupported region, and missing FUSE prerequisites return actionable errors.
- `rg UseDrive9Companion internal` returns no matches, and there is no equivalent runtime switch that can select tdc-native fs behavior.
- `make test` does not include native fs HTTP/FUSE/WebDAV behavior tests as tdc product-contract tests; it covers wrapper responsibilities with a fake companion.
- Live e2e verifies the wrapper path and includes the overwrite-existing-file regression.
- README, AGENTS.md, help text, and installer next steps match the wrapper-based behavior.

## Out Of Scope

- Importing Drive9 Go packages into tdc.
- Building tdc against `ref/drive9` or any local `ref/` path.
- Rewriting Drive9 FUSE, WebDAV, journal, vault, git, layer, pack, or upload internals inside tdc.
- Supporting arbitrary user-installed Drive9 versions without a compatibility check.
- Fully normalizing all Drive9 command output into tdc JSON in the first wrapper iteration.
- Mutating or deleting a user's standalone `drive9` installation unless a future explicit migration flag is designed.
