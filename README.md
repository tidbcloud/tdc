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

Configure the `tdc` with the TiDB Cloud Public Key and Private Key from the [TiDB Cloud](https://tidbcloud.com/org-settings/api-keys) console. The available region codes are: `aws-us-east-1`, `aws-us-west-2`, and `aws-ap-southeast-1`.

```shell
tdc configure --non-interactive --region-code <TDC_REGION_CODE> --tdc-public-key <TDC_PUBLIC_KEY> --tdc-private-key <TDC_PRIVATE_KEY>
```

### TiDB Cloud Filesystem

```shell
mkdir ~/my-workspace
tdc fs create-file-system --file-system-name my-workspace
tdc fs mount-file-system --file-system-name my-workspace --mount-path ~/my-workspace
```

### TiDB Cloud Starter

```shell
tdc db create-db-cluster --db-cluster-name my-distributed-mysql --db-cluster-type starter
```

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
