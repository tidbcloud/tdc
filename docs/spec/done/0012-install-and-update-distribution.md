# Install And Update Distribution

## Goal

Make `tdc` installable and updatable through deterministic GitHub Releases artifacts. The MVP channel is GoReleaser plus GitHub Releases, with shell and PowerShell installers. Homebrew and Scoop are intentionally deferred to `docs/spec/0016-homebrew-and-scoop-distribution.md`.

## User-facing Commands

- `tdc cli check-update`
- `tdc cli check-update --fail-if-update-available`
- `tdc cli update --dry-run`
- `tdc cli update --yes`
- `tdc cli update --target-version v0.1.0 --yes`

External installers:

- `scripts/install.sh --version latest --yes`
- `scripts/install.sh --version latest --install-dir "$HOME/.local/bin" --yes`
- `scripts/install.ps1 -Version latest -InstallDir "$HOME\bin" -Yes`

The installer scripts use `--version` because they are not Cobra commands. The `tdc cli update` command uses `--target-version` so it does not shadow the global `--version` flag that works at every CLI level.

## Behavior

- Releases are published by GoReleaser to GitHub Releases in `github.com/Icemap/tdc`.
- Release assets use stable names scoped by GitHub tag:
  - `tdc_darwin_arm64.tar.gz`
  - `tdc_darwin_amd64.tar.gz`
  - `tdc_linux_arm64.tar.gz`
  - `tdc_linux_amd64.tar.gz`
  - `tdc_windows_amd64.zip`
  - `tdc_checksums.txt`
  - `install.sh`
  - `install.ps1`
- `scripts/install.sh` and `scripts/install.ps1` download archives from GitHub Releases, verify `tdc_checksums.txt`, extract `tdc`, install it into the selected directory, and run `tdc --version`.
- Install scripts support pinned versions and `latest`. For `latest`, they use `https://github.com/Icemap/tdc/releases/latest/download/<asset>`. For pinned versions, they use `https://github.com/Icemap/tdc/releases/download/<tag>/<asset>`.
- Install scripts prefer upgrading the active `tdc`/`tdc.exe` binary found on `PATH` unless `--install-dir` or `TDC_INSTALL_DIR` is set.
- On macOS/Linux, if no active binary exists and no install directory is supplied, `scripts/install.sh` installs to `/usr/local/bin`. It creates the directory or moves the binary with `sudo` when the current user cannot write there.
- Install scripts detect PATH shadowing after installation and report both the installed path and the path currently resolved by `PATH`.
- Install scripts bootstrap `~/.tdc/config` only when it is missing, writing a default `[default]` profile with `cloud_provider = 'aws'` and `region_code = 'us-east-1'`. They do not write `~/.tdc/credentials`.
- Install scripts print DB config regions, fetch and print tdc fs regions from `https://drive9.ai/manifest/regions/drive9-regions.json` when available, and finish with clear next-step commands.
- `tdc cli check-update` calls the GitHub Releases API `GET /repos/Icemap/tdc/releases/latest`, matches the current OS/arch release asset, compares the local version with the latest tag, and prints structured output.
- `tdc cli update` updates only binaries built with `install_source=archive` or `install_source=script`. Local builds and unknown installs are refused. Package-manager installs are refused with package-manager-specific guidance.
- `tdc cli update --dry-run` resolves the target release, artifact, checksum, and target path, but does not download or replace the binary.
- `tdc cli update --yes` downloads the target archive and `tdc_checksums.txt`, verifies SHA-256, extracts the binary, atomically replaces the current binary on Unix-like platforms, and validates the new binary by running `tdc --version`.
- Windows self-update cannot safely replace the running executable yet. On Windows, `tdc cli check-update` and install scripts are supported; `tdc cli update --yes` returns an actionable error telling the user to rerun the PowerShell installer.
- No command performs background or silent auto-update.
- Update never reads, modifies, or uploads `~/.tdc/config`, `~/.tdc/credentials`, DB SQL credentials, tdc fs credentials, SQL text, file contents, or API response payloads.

## Inputs And Config

`tdc cli check-update` flags:

- `--output json|human`
- `--query <jmespath-expression>`
- `--fail-if-update-available`

`tdc cli update` flags:

- `--target-version <latest|vX.Y.Z>`, default `latest`
- `--dry-run`
- `--yes`
- `--output json|human`
- `--query <jmespath-expression>`

Build metadata exposed through `internal/version`:

- version
- git commit
- build date
- OS/arch
- install source
- release channel

Install source values:

- `archive`: GoReleaser/GitHub Releases binary, owned by `tdc cli update`
- `script`: script-installed binary, owned by `tdc cli update` if used later
- `local`: `make build` or local developer build, not update-owned
- `homebrew`, `scoop`, `winget`: package-manager installs, not update-owned
- `unknown`: not update-owned

`TDC_RELEASE_API_BASE_URL` exists only as a test override for fake GitHub Releases servers in unit/e2e tests. It is not normal user configuration and must not be documented as a required user workflow.

## Output And Errors

`tdc cli check-update --output json` output shape:

```json
{
  "current_version": "0.1.0",
  "latest_version": "0.1.1",
  "update_available": true,
  "current_version_comparable": true,
  "install_source": "archive",
  "release_channel": "stable",
  "artifact_name": "tdc_darwin_arm64.tar.gz",
  "download_url": "https://github.com/Icemap/tdc/releases/download/v0.1.1/tdc_darwin_arm64.tar.gz",
  "release_url": "https://github.com/Icemap/tdc/releases/tag/v0.1.1",
  "release_notes_url": "https://github.com/Icemap/tdc/releases/tag/v0.1.1"
}
```

`tdc cli update --dry-run --output json` output shape:

```json
{
  "current_version": "0.1.0",
  "target_version": "0.1.1",
  "updated": false,
  "dry_run": true,
  "install_source": "archive",
  "release_channel": "stable",
  "artifact_name": "tdc_darwin_arm64.tar.gz",
  "download_url": "https://github.com/Icemap/tdc/releases/download/v0.1.1/tdc_darwin_arm64.tar.gz",
  "checksum_sha256": "<hex-sha256>",
  "target_path": "/Users/me/.local/bin/tdc",
  "release_url": "https://github.com/Icemap/tdc/releases/tag/v0.1.1",
  "release_notes_url": "https://github.com/Icemap/tdc/releases/tag/v0.1.1"
}
```

Stable error codes:

- `update.available`: `check-update --fail-if-update-available` found a newer release
- `update.confirmation_required`: `tdc cli update` was run without `--yes` or `--dry-run`
- `update.managed_install`: Homebrew/Scoop/Winget or another package manager owns the install
- `update.unknown_install`: local, unknown, or otherwise not update-owned install
- `update.permission_denied`: target path or directory cannot be replaced by the current user
- `update.network_error`: GitHub release metadata, checksum, or artifact download failed
- `update.release_not_found`: requested release tag was not found
- `update.artifact_not_found`: the release does not include the OS/arch asset
- `update.checksum_missing`: `tdc_checksums.txt` does not contain the selected asset
- `update.checksum_mismatch`: downloaded archive hash did not match `tdc_checksums.txt`
- `update.no_update_available`: requested target is already installed
- `update.unsupported_platform`: actual self-update is not available on the current platform
- `update.unsupported_target`: no release asset is defined for the current OS/arch
- `update.validation_failed`: the replaced binary failed `tdc --version`

## After This Spec

Users and agents can install from GitHub Releases:

```bash
curl -fsSL https://github.com/Icemap/tdc/releases/latest/download/install.sh | sh -s -- --yes
tdc --version
tdc cli check-update --output json
tdc cli update --dry-run
tdc cli update --yes
```

Pinned install:

```bash
curl -fsSL https://github.com/Icemap/tdc/releases/download/v0.1.0/install.sh | sh -s -- --version v0.1.0 --yes
```

Windows users can run the PowerShell installer:

```powershell
$script = "$env:TEMP\install-tdc.ps1"
iwr https://github.com/Icemap/tdc/releases/latest/download/install.ps1 -OutFile $script
powershell -ExecutionPolicy Bypass -File $script -InstallDir "$HOME\bin" -Yes
tdc --version
```

## Implementation Design

- `.goreleaser.yaml` defines cgo-free Go builds for supported OS/arch pairs, stable archive names, `tdc_checksums.txt`, and GitHub release extra files for `install.sh` and `install.ps1`.
- `.github/workflows/release.yml` runs GoReleaser on `v*` tags with `contents: write`. This workflow is intentionally only release publishing; full CI/CD remains in `0014`.
- `scripts/install.sh` supports macOS/Linux `amd64` and `arm64`.
- `scripts/install.ps1` supports Windows `amd64`.
- `internal/version` carries release metadata through Go linker variables.
- `internal/update` owns GitHub release lookup, semantic version comparison, artifact selection, checksum verification, archive extraction, install-source checks, and atomic Unix replacement.
- `internal/cli` wires `tdc cli check-update` and `tdc cli update` as normal structured-output commands.
- Release builds set `installSource=archive`; local Makefile builds set `installSource=local`.

## API Call Chain

This spec adds no TiDB Cloud product API calls.

`tdc cli check-update`:

1. Read local version metadata.
2. `GET https://api.github.com/repos/Icemap/tdc/releases/latest`.
3. Select the release asset matching current OS/arch.
4. Compare local version and latest release tag.
5. Render JSON or human output.

`tdc cli update --dry-run`:

1. Resolve current executable path and install source.
2. Refuse package-managed or unknown installs.
3. `GET https://api.github.com/repos/Icemap/tdc/releases/latest` or `GET https://api.github.com/repos/Icemap/tdc/releases/tags/<tag>`.
4. Select the matching archive and `tdc_checksums.txt`.
5. Download only `tdc_checksums.txt`.
6. Render the planned artifact, checksum, and target path.

`tdc cli update --yes`:

1. Run the dry-run resolution path.
2. Download the selected archive.
3. Verify SHA-256 against `tdc_checksums.txt`.
4. Extract `tdc` or `tdc.exe`.
5. Replace the current binary on Unix-like platforms.
6. Run `tdc --version` on the new binary.

Installer scripts:

1. Detect OS and arch.
2. Resolve the install directory from `--install-dir`, `TDC_INSTALL_DIR`, the active `tdc`/`tdc.exe` on `PATH`, or the platform default.
3. Compose deterministic GitHub Releases download URLs.
4. Download archive and `tdc_checksums.txt`.
5. Verify SHA-256.
6. Extract and install the binary, using `sudo` on macOS/Linux only when needed.
7. Run `tdc --version`.
8. Bootstrap `~/.tdc/config` if missing.
9. Report PATH shadowing, supported DB regions, supported tdc fs regions, and next steps.

## Dependencies And Platform

- Runtime implementation uses only the Go standard library.
- Release automation uses GoReleaser as a release-time dependency, not a runtime dependency.
- GitHub Actions uses `goreleaser/goreleaser-action`.
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

- `make build` embeds local install-source metadata.
- `make test` passes without live credentials.
- `make e2e` passes without live credentials and uses a fake release server for update checks.
- GoReleaser snapshot builds can be generated from a clean checkout with `make release-snapshot` when GoReleaser is installed.
- Install scripts support pinned version and latest installs.
- Install scripts verify `tdc_checksums.txt`.
- Install scripts upgrade the active binary by default and detect PATH shadowing.
- Install scripts bootstrap missing `~/.tdc/config` without touching credentials.
- Install scripts print DB and tdc fs region lists plus clear next steps.
- `tdc cli check-update` reports update availability without reading credentials.
- `tdc cli update --dry-run` reports the planned update without changing files.
- `tdc cli update --yes` updates an owned archive/script install on Unix-like platforms.
- `tdc cli update` refuses package-managed, local, unknown, and non-writable installs with actionable guidance.
- README documents install/update commands and the current package-manager deferral.

## Out Of Scope

- Background update checks.
- Silent auto-update.
- Updating TiDB Cloud credentials or DB SQL credentials.
- Config migrations that modify user config during update.
- Homebrew tap and Scoop bucket publishing. See `0016-homebrew-and-scoop-distribution.md`.
- Linux apt/yum repositories.
- Winget publishing.
- Notarization or binary signing beyond SHA-256 checksums for MVP.
