# Starter DB Cluster Lifecycle

## Goal

Provide Starter cluster lifecycle management under `tdc db`.

## User-facing Commands

Initial command set:

- `tdc db create-db-cluster`
- `tdc db list-db-clusters`
- `tdc db describe-db-cluster`
- `tdc db update-db-cluster`
- `tdc db delete-db-cluster`

Primary create shape:

```bash
tdc db create-db-cluster --db-cluster-name <name> --db-cluster-type starter
```

## Behavior

- `tdc db` manages TiDB Cloud Starter database clusters.
- Require `--db-cluster-type starter` for create to preserve future tier
  compatibility.
- Use long flags only.
- Mutating commands support `--dry-run`.
- Commands must not prompt.
- Delete must require an explicit long flag confirmation pattern or another
  non-interactive safety mechanism; it must never prompt.

## Inputs And Config

- Requires credentials and region routing.
- Common identifiers should use explicit names such as `--db-cluster-id` and
  `--db-cluster-name`.
- Optional create parameters should map directly to available Starter API
  fields. Do not invent unsupported fields.

## Output And Errors

- JSON is the default output.
- Create and update return the remote resource representation or operation
  status returned by the API.
- Delete returns a structured confirmation or operation status.
- Errors should distinguish validation failures, authentication failures,
  permission failures, not found, quota/capacity issues, and backend API errors.
- Permission errors must name the required permission, such as
  `starter.cluster.create`, without exposing credentials.

## After This Spec

Users can create and manage Starter clusters in the configured cloud provider
and region:

```bash
tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --dry-run
tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter
tdc db list-db-clusters --query 'clusters[].id'
tdc db describe-db-cluster --db-cluster-id <id>
tdc db delete-db-cluster --db-cluster-id <id> --confirm-db-cluster-name demo
```

No command asks for a server URL. The active profile's `cloud_provider` and
`region_code` decide the target Starter API endpoint.

## Implementation Design

- `internal/cli/db` owns `tdc db` command registration and flag validation.
- `internal/db` owns cluster lifecycle use cases and declares authorization
  requirements for read/create/update/delete actions.
- `internal/api/starter` contains Starter cluster HTTP request/response models.
- `internal/db/validate` normalizes cluster names, cluster type, spending limit,
  and optional API-backed create parameters.
- `internal/dryrun` is used by create/update/delete to return validated request
  summaries without sending mutating HTTP requests.
- Delete safety is implemented as an explicit flag, initially
  `--confirm-db-cluster-name <name>`, so it remains non-interactive.

## API Call Chain

Confirmed API base:

- `https://serverless.tidbapi.com`
- HTTP Digest auth with TiDB Cloud public/private API keys.

Command mapping:

- `tdc db list-db-clusters`
  1. Validate profile, provider, and region.
  2. Call `GET /v1beta1/clusters` with optional `filter`, `pageSize`,
     `pageToken`, `orderBy`, and `skip`.
  3. Filter or validate Starter-only behavior using returned cluster metadata.
- `tdc db create-db-cluster`
  1. Validate `--db-cluster-type starter`.
  2. Optionally call `GET /v1beta1/regions` to confirm the active
     provider/region is available to the organization.
  3. Call `POST /v1beta1/clusters` with `displayName`, project label, region
     name such as `regions/aws-us-east-1`, and only other confirmed fields.
- `tdc db describe-db-cluster`
  1. Call `GET /v1beta1/clusters/{clusterId}`.
- `tdc db update-db-cluster`
  1. Call `PATCH /v1beta1/clusters/{cluster.clusterId}` with `updateMask` and
     the supported cluster fields being updated.
- `tdc db delete-db-cluster`
  1. Validate `--confirm-db-cluster-name`.
  2. Call `DELETE /v1beta1/clusters/{clusterId}`.

Available but not part of this lifecycle MVP:

- `PUT /v1beta1/clusters/{clusterId}/password` for root password changes.
- `POST /v1beta1/clusters:batchCreate`.
- `GET /v1beta1/clusters:batchGet`.

## Dependencies And Platform

- No new third-party dependency beyond specs `0001` through `0004`.
- Uses the shared authenticated HTTP client and output/query layer.
- No cgo is required.
- Platform-neutral.

## Dependencies

- `0004-api-client-auth-and-region-routing.md`.
- `0003-output-error-query-dry-run.md`.

## Acceptance Criteria

- Mock API tests cover create/list/describe/update/delete.
- Tests cover required `--db-cluster-type starter` on create.
- Tests cover dry-run request validation without sending mutating requests.
- Tests cover stable JSON output and `--query`.
- Tests cover delete safety behavior without prompts.

## Out Of Scope

- Dedicated or Premium cluster-specific behavior.
- SQL user preparation and SQL execution, covered by
  `0008-starter-db-sql-access-and-query.md`.
- Import, export, backup, and audit-log commands unless added by a later spec.
