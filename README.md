# tdc

`tdc` is the command-line interface for TiDB Cloud Filesystem and TiDB Cloud Starter.

- TiDB Cloud Filesystem is a distributed file system designed specifically for AI coding agent workloads, with zero infrastructure.
- TiDB Cloud Starter provides distributed database clusters that are fully compatible with MySQL, with zero infrastructure.

## Your Agent's Toolbelt

### Always-on, zero infrastructure file system for sandboxes — The 3-Command Superpower

An agent persist state between sessions, share files across sandboxes, snapshot its workspace before attempting a risky operation, and roll back on failure — all through a CLI with POSIX compatibility.

1. Create a filesystem resource and get the returning token (one-time, out of the sandbox)

```shell
TDC_FS_TOKEN=$(tdc fs create-file-system --file-system-name agent-workspace --region <REGION_CODE>)
```

2. Mount the filesystem and use just like any regular POSIX-compliant filesystem (inside the sandbox environment)

```shell
export TDC_FS_TOKEN="<FS_TOKEN>"
tdc fs mount-file-system --file-system-name agent-workspace --mount-path /path_to_workspace --region <REGION_CODE>
echo "Hello Sandbox Workspace!" >> /path_to_workspace/hello.txt
```

3. Unmount to safely release the workspace before handing off to another sandbox (inside the sandbox environment)

```shell
tdc fs unmount-file-system --mount-path /path_to_workspace --region <REGION_CODE>
```

### Always-on, zero infrastructure MySQL — The 3-Command Superpower

An agent can go from zero to live HTAP SQL (Hybrid Transaction / Analytical Processing) in three commands:

1. Provision a serverless MySQL-compatible cluster (~15 seconds)

```shell
tdc db create-db-cluster --db-cluster-type starter --db-cluster-name my-app-db
```

2. Create the SQL users it needs to connect

```shell    
tdc db create-db-sql-users --db-cluster-id <ID>
```

3. Retrieve the database connection string for your agent and share it across sandboxes as needed

```shell
DATABASE_URL=$(tdc db format-db-connection-string --db-cluster-id <ID> --read-write --query "connection_string")
```

## Install

macOS and Linux users:

```bash
curl -fsSL https://github.com/tidbcloud/tdc/releases/latest/download/install.sh | sh -s -- --yes
export PATH="$HOME/.tdc/bin:$PATH"
tdc --version
```

The installer writes `tdc` and `tdc-drive9` to `~/.tdc/bin` without sudo. Add the `export PATH=...` line to your shell profile to make it persistent.

Windows users:

```powershell
$script = "$env:TEMP\install-tdc.ps1"
iwr https://github.com/tidbcloud/tdc/releases/latest/download/install.ps1 -OutFile $script
powershell -ExecutionPolicy Bypass -File $script -Yes
$env:Path = "$HOME\.tdc\bin;$env:Path"
tdc --version
```

## Quick Start Guide

### Configure

Configure `tdc` with a TiDB Cloud Public Key and Private Key from the [TiDB Cloud](https://tidbcloud.com/org-settings/api-keys) console. Supported region codes are `aws-us-east-1`, `aws-us-west-2`, `aws-eu-central-1`, `aws-ap-northeast-1`, `aws-ap-southeast-1`, and `ali-ap-southeast-1`.

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

Supported regions: `aws-us-east-1` and `aws-ap-southeast-1`.

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
```

`create-file-system` returns an file system token (`fs_token`) in its JSON result. This is the file system owner credential and should be handled as a secret. A configured machine can provision a file system and capture the token without printing the full result:

```shell
export TDC_FS_TOKEN="$(tdc fs create-file-system --file-system-name agent-workspace --query fs_token --output text)"
```

An agent sandbox can then use that existing file system without running `tdc configure` or providing TiDB Cloud API keys:

```shell
export TDC_FS_TOKEN="<FS_TOKEN>"
tdc fs mount-file-system --file-system-name agent-workspace --mount-path /path_to_workspace --region aws-us-east-1
```

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
tdc update
tdc update --target-version v0.1.1
```

`tdc update` downloads and verifies both `tdc` and its `tdc-drive9` companion before replacing either binary in the user-writable install directory. It never requests sudo. Legacy installations under `/usr/local/bin` must run the installer once to migrate to `~/.tdc/bin`.

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

## Test

Run local unit and black-box tests without live cloud credentials:

```bash
make test
make e2e
```

Run one live command family against the `live-e2e` profile:

```bash
make live-e2e-configure
make live-e2e-organization
make live-e2e-db
make live-e2e-fs
make live-e2e-fs-git
make live-e2e-fs-journal
make live-e2e-fs-vault
```

Run the complete live suite in one test process:

```bash
make live-e2e
```

Set `LIVE_E2E_PROFILE=<profile>` to use a profile other than `live-e2e`. The DB and FS suites perform real cloud mutations and clean up only resources created by the test run.
