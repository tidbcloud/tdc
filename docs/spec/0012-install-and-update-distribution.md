# Install And Update Distribution

## Goal

Make `tdc` installable and updatable in a deterministic, agent-friendly, and
cross-platform way.

## User-facing Commands

The CLI exposes update operations under a `cli` command namespace:

- `tdc cli check-update`
- `tdc cli update`

Installation is primarily external because the binary is not available before
install:

- macOS/Linux install script
- Windows PowerShell install script
- direct release archive download
- package-manager channels after beta validation

## Behavior

- The release artifact is the source of truth. Each release publishes static
  binaries for supported OS/arch pairs plus checksums.
- Default install is explicit and version-pinnable. Users and agents can install
  `latest` or a specific version.
- No command performs background or silent auto-update.
- `tdc cli check-update` checks the release manifest and prints JSON by default.
- `tdc cli update` updates only installs that `tdc` can safely own.
- Package-manager installs are not modified by `tdc cli update`; the command
  returns an actionable error with the package-manager upgrade command.
- Unknown or non-writable installs are not modified unless a future explicit
  recovery workflow is added.
- Update never reads, modifies, or uploads `~/.tdc/credentials`.
- Update does not migrate user config by default. If a future release requires a
  config migration, it must be a separate explicit command or a clearly reported
  startup-time compatibility check.

## Install Channels

MVP install channels:

1. Direct release archives from the product release location.
2. macOS/Linux install script.
3. Windows PowerShell install script.
4. `go install` for contributors and source-based testing only.

Post-MVP install channels:

1. Homebrew tap for macOS/Linux.
2. Winget and Scoop for Windows.
3. Linux package repositories only after the binary and mount story stabilize.

`go install` is useful for developers but should not be the primary user-facing
install path because release metadata, checksums, signatures, and install-source
detection are weaker than the official artifacts.

## Inputs And Config

Install scripts support:

- `--version <version>` with `latest` as the default
- `--channel stable|beta`
- `--install-dir <path>`
- `--dry-run`
- `--yes`

`tdc cli check-update` supports:

- `--channel stable|beta`
- `--output json|human`
- `--fail-if-update-available` for CI and automation

`tdc cli update` supports:

- `--version <version>` with `latest` as the default
- `--channel stable|beta`
- `--dry-run`
- `--yes`

The binary exposes build metadata through `internal/version`:

- version
- git commit
- build date
- OS/arch
- install source when known
- release channel when known

Install-source handling:

- Official release archives use `install_source=archive`.
- Official install scripts use release archives and may record local
  non-secret install metadata under `~/.tdc/config` if needed.
- Package-manager formulas should build or wrap with
  `install_source=homebrew`, `install_source=winget`, or another explicit
  source marker.
- `go install` and locally built binaries are treated as `unknown` unless
  build metadata says otherwise.

## Output And Errors

`tdc cli check-update` JSON output shape:

```json
{
  "current_version": "0.1.0",
  "latest_version": "0.1.1",
  "update_available": true,
  "channel": "stable",
  "install_source": "archive",
  "download_url": "https://example.invalid/tdc/releases/0.1.1/tdc_0.1.1_darwin_arm64.tar.gz",
  "release_notes_url": "https://example.invalid/tdc/releases/0.1.1"
}
```

`tdc cli update --dry-run` prints the planned artifact, checksum, target path,
and install source without modifying the filesystem.

`tdc cli update` must fail with stable error codes:

- `update.managed_install`: binary is managed by Homebrew, Winget, Scoop, or
  another package manager.
- `update.unknown_install`: install source is unknown.
- `update.permission_denied`: target binary path is not writable.
- `update.checksum_mismatch`: downloaded artifact failed verification.
- `update.network_error`: manifest or artifact download failed.
- `update.no_update_available`: requested update is already installed.

Error messages must include the next command when known, for example
`brew upgrade tdc` for Homebrew installs.

## Release Manifest Contract

The release manifest endpoint is product-owned and not user-configurable in
normal operation. Tests may override it through a test-only environment variable.

Required endpoints:

- `GET <release-base-url>/channels/<channel>/latest.json`
- `GET <release-base-url>/versions/<version>/manifest.json`

Manifest shape:

```json
{
  "channel": "stable",
  "latest_version": "0.1.1",
  "minimum_supported_version": "0.1.0",
  "release_notes_url": "https://example.invalid/tdc/releases/0.1.1",
  "artifacts": [
    {
      "os": "darwin",
      "arch": "arm64",
      "archive_type": "tar.gz",
      "url": "https://example.invalid/tdc/releases/0.1.1/tdc_0.1.1_darwin_arm64.tar.gz",
      "sha256": "<hex-sha256>",
      "signature_url": "https://example.invalid/tdc/releases/0.1.1/tdc_0.1.1_darwin_arm64.tar.gz.sig"
    }
  ]
}
```

The manifest contains no user, organization, project, cluster, or credential
data. Clients send only normal HTTP metadata plus `User-Agent: tdc/<version>`.

## After This Spec

Users and agents can install and update deterministically:

```bash
curl -fsSL https://example.invalid/tdc/install.sh | sh -s -- --version 0.1.0 --install-dir "$HOME/.local/bin"
tdc --version
tdc cli check-update --output json
tdc cli update --dry-run
tdc cli update --yes
```

Windows users can use the equivalent PowerShell installer:

```powershell
irm https://example.invalid/tdc/install.ps1 | iex
tdc --version
tdc cli check-update --output json
```

The placeholder release URLs must be replaced by the final product release
location before implementation.

## Implementation Design

- `internal/cli/cli` registers CLI-management commands.
- `internal/update` owns manifest fetching, semver comparison, artifact
  selection, checksum verification, and atomic replacement.
- `internal/version` exposes build metadata and install-source markers.
- `scripts/install.sh` installs on macOS/Linux.
- `scripts/install.ps1` installs on Windows.
- Release automation builds archives named
  `tdc_<version>_<os>_<arch>.tar.gz` for Unix-like platforms and
  `tdc_<version>_<os>_<arch>.zip` for Windows.
- Release automation publishes `checksums.txt` and a signed checksum file.
- Self-update uses an atomic replace strategy:
  1. download to a temp file in the same filesystem when possible
  2. verify checksum and signature
  3. extract and validate `tdc --version`
  4. rename current binary to a backup path
  5. move the new binary into place
  6. remove the backup after the new binary starts successfully
- The updater must not call `sudo`. If the install path needs elevated
  privileges, return `update.permission_denied` with instructions.

Package-manager installs:

- Homebrew formula should run `brew upgrade tdc`.
- Winget package should run `winget upgrade <package-id>`.
- Scoop manifest should run `scoop update tdc`.
- `tdc cli update` detects those sources through build metadata first and
  known install paths second.

## API Call Chain

This spec adds no TiDB Cloud product API calls.

Install script call chain:

1. Detect OS and arch.
2. Fetch `GET <release-base-url>/channels/<channel>/latest.json` or
   `GET <release-base-url>/versions/<version>/manifest.json`.
3. Select the artifact matching OS and arch.
4. Download the selected archive.
5. Download or read the checksum and signature from the manifest.
6. Verify artifact.
7. Install the `tdc` binary into the selected install directory.
8. Run `tdc --version`.

`tdc cli check-update` call chain:

1. Read local version metadata.
2. Fetch `GET <release-base-url>/channels/<channel>/latest.json`.
3. Compare semantic versions.
4. Print JSON or human output.

`tdc cli update` call chain:

1. Read local version and install source.
2. Refuse package-managed, unknown, or non-writable installs.
3. Fetch `GET <release-base-url>/channels/<channel>/latest.json` or
   `GET <release-base-url>/versions/<version>/manifest.json`.
4. Select the artifact matching OS and arch.
5. Download and verify the selected artifact.
6. Replace the binary atomically.
7. Run the new `tdc --version` as a post-update validation.

## Dependencies And Platform

- Runtime dependencies should stay within the Go standard library when
  practical.
- Release automation may use GoReleaser or equivalent tooling, but this is a
  release-time dependency, not a runtime dependency.
- Signature verification may use a small pure-Go dependency if needed; it must
  not require cgo.
- Default release binaries are built with `CGO_ENABLED=0`.
- Supported MVP artifacts:
  - `darwin/arm64`
  - `darwin/amd64`
  - `linux/arm64`
  - `linux/amd64`
  - `windows/amd64`
- Build, test, release, and package steps must exclude `ref/`.

## Dependencies

- `0001-cli-foundation.md`
- `0002-local-config-and-credentials.md`

## Acceptance Criteria

- Release archives can be built from a clean checkout.
- Install scripts can install a pinned version.
- Install scripts verify checksums and signatures before installing.
- `tdc cli check-update` reports update availability without reading
  credentials.
- `tdc cli update --dry-run` reports the planned update without changing files.
- `tdc cli update --yes` updates an owned archive/script install.
- `tdc cli update` refuses package-managed installs with actionable guidance.
- `tdc cli update` refuses unknown or non-writable installs.
- All update commands return JSON-compatible structured errors.
- `go test ./...` passes.

## Out Of Scope

- Background update checks.
- Silent auto-update.
- Updating TiDB Cloud credentials or DB SQL credentials.
- Config migrations that modify user config during update.
- Linux apt/yum repositories for MVP.
