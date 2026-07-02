# Starter DB Branch Lifecycle

## Goal

Expose Starter cluster branch operations needed for database development and
agent workflows.

## User-facing Commands

Initial command set:

- `tdc db create-db-cluster-branch`
- `tdc db list-db-cluster-branches`
- `tdc db describe-db-cluster-branch`
- `tdc db delete-db-cluster-branch`

## Behavior

- Branch commands live under `tdc db` and remain two-level commands.
- Mutating branch commands support `--dry-run`.
- Commands must not prompt.
- Branch operations require explicit cluster identification.
- Delete must use a non-interactive safety mechanism.

## Inputs And Config

Common flags:

- `--db-cluster-id <id>`
- `--db-cluster-branch-id <id>` where required
- `--db-cluster-branch-name <name>` where supported by the API

Use only fields supported by the Starter branch API.

## Output And Errors

- JSON is the default output.
- Branch create returns the branch resource or operation status.
- Not-found errors should identify whether the missing object is the cluster or
  the branch.
- Permission errors must name the required permission, such as
  `starter.branch.create`.

## After This Spec

Users can manage Starter branch workflows without adding another command level:

```bash
tdc db create-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-name dev --dry-run
tdc db list-db-cluster-branches --db-cluster-id <cluster-id>
tdc db describe-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-id <branch-id>
tdc db delete-db-cluster-branch --db-cluster-id <cluster-id> --db-cluster-branch-id <branch-id> --confirm-db-cluster-branch-name dev
```

Branches use the same provider/region routing as their parent Starter cluster
commands.

## Implementation Design

- `internal/cli` registers branch subcommands next to cluster subcommands.
- `internal/db` owns branch use cases and authorization requirements.
- `internal/api/starter` adds branch endpoints and response models under the
  same Starter client.
- `internal/db/validate` validates cluster and branch identifiers.
- Delete safety is implemented through an explicit long flag, initially
  `--confirm-db-cluster-branch-name <name>`.

## API Call Chain

Confirmed API base:

- `https://serverless.tidbapi.com`
- HTTP Digest auth with TiDB Cloud public/private API keys.

Command mapping:

- `tdc db list-db-cluster-branches`
  1. Validate `--db-cluster-id`.
  2. Call `GET /v1beta1/clusters/{clusterId}/branches` with optional
     `pageSize` and `pageToken`.
- `tdc db create-db-cluster-branch`
  1. Validate `--db-cluster-id` and API-backed branch fields such as
     `--db-cluster-branch-name`.
  2. Call `POST /v1beta1/clusters/{clusterId}/branches`.
- `tdc db describe-db-cluster-branch`
  1. Call `GET /v1beta1/clusters/{clusterId}/branches/{branchId}`.
- `tdc db delete-db-cluster-branch`
  1. Validate `--confirm-db-cluster-branch-name`.
  2. Call `DELETE /v1beta1/clusters/{clusterId}/branches/{branchId}`.

Available but out of scope for this spec:

- `POST /v1beta1/clusters/{clusterId}/branches/{branchId}:reset`.
- `PATCH /v1beta1/clusters/{clusterId}/branches/{branchId}/sqlUsers/{username}`.

## Dependencies And Platform

- No new third-party dependency beyond previous specs.
- Uses the existing Starter API client.
- No cgo is required.
- Platform-neutral.

## Dependencies

- `0006-starter-db-cluster-lifecycle.md`.

## Acceptance Criteria

- Mock API tests cover create/list/describe/delete.
- Tests cover required cluster identification.
- Tests cover dry-run for create and delete.
- Tests cover branch not-found errors.
- Tests cover `--query` over list output.
- `make live-e2e` creates a temporary `tdc-e2e-branch-*` branch on the
  temporary `tdc-e2e-*` Starter cluster, then lists, describes, deletes, and
  verifies deletion for that branch without touching pre-existing branches.

## Out Of Scope

- Branch shell access.
- Branch reset unless product principles are updated to include it.
- Cross-cluster branch operations.
