# tdc


`tdc` is the command-line interface for TiDB Cloud Filesystem and TiDB Cloud Starter.

- TiDB Cloud Filesystem is a distributed file system designed specifically for AI coding agent workloads.
- TiDB Cloud Starter provides distributed database clusters that are fully compatible with MySQL.

## Install

macOS and Linux users:

```bash
curl -fsSL https://github.com/tidbcloud/tdc/releases/latest/download/install.sh | sh -s -- --yes
```

Windows users:

```powershell
$script = "$env:TEMP\install-tdc.ps1"
iwr https://github.com/tidbcloud/tdc/releases/latest/download/install.ps1 -OutFile $script
powershell -ExecutionPolicy Bypass -File $script -InstallDir "$HOME\bin" -Yes
```

## Quick Start Guide

### Configure

Configure tdc with a TiDB Cloud Public Key and Private Key from the [TiDB Cloud](https://tidbcloud.com/org-settings/api-keys) console. Supported region codes are `aws-us-east-1`, `aws-us-west-2`, `aws-eu-central-1`, `aws-ap-northeast-1`, `aws-ap-southeast-1`, and `ali-ap-southeast-1`.

```shell
tdc configure --non-interactive --region-code <TDC_REGION_CODE> --tdc-public-key <TDC_PUBLIC_KEY> --tdc-private-key <TDC_PRIVATE_KEY>
```

Configure verifies the API key by listing all accessible projects, requires exactly one project with `type = "tidbx_virtual"`, and stores its ID as the profile's default `project_id` in `~/.tdc/config`. API credentials remain in `~/.tdc/credentials`. Configuration fails without changing the profile when project discovery fails.

```toml
[default]
region_code = "aws-us-east-1"
project_id = "1372813089454645969"
```

### TiDB Cloud Filesystem

```shell
mkdir ~/my-workspace
tdc fs create-file-system --file-system-name my-workspace
tdc fs mount-file-system --mount-path ~/my-workspace
```

One profile can manage multiple file systems. The first created file system becomes the profile default; later resources can be selected explicitly or made the default:

```shell
tdc fs create-file-system --file-system-name scratch
tdc fs list-file-systems
tdc fs describe-file-system --file-system-name scratch
tdc fs set-default-file-system --file-system-name scratch
tdc fs list-files --path /
tdc fs list-files --file-system-name my-workspace --path /
tdc fs unset-default-file-system
```

Resource selection uses `--file-system-name`, then `TDC_FS_FILE_SYSTEM_NAME`, then the profile default, then the only configured resource. Commands fail with an ambiguity error when multiple resources exist and none is selected. File system metadata and credentials are isolated under `~/.tdc/fs_resources/<profile-key>/<resource-key>/`; API keys are never printed by list or describe commands.

### TiDB Cloud Starter

```shell
tdc db create-db-cluster --db-cluster-name my-distributed-mysql --db-cluster-type starter
```

Cluster creation uses the configured `project_id` by default. Use optional `--project-id <project-id>` to create in another accessible project. An explicit empty `--project-id` is rejected instead of falling back to the profile.

### Organization Projects

```shell
tdc organization list-projects
```

Each project includes a `type`: `tidbx` identifies a regular project and `tidbx_virtual` identifies a virtual project.

## Update

```bash
tdc update --check
tdc update --dry-run
tdc update --yes
tdc update --target-version v0.1.0 --yes
```

## Build from source

Requirements:

- Go 1.26.5 or newer
- `make`
- GoReleaser, only for `make release-snapshot` or release publishing

Build the local binary:

```bash
make build
```

The binary is written to:

```text
bin/tdc
```
