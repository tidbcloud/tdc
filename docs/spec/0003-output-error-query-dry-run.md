# Output, Errors, Query, And Dry Run

## Goal

Define shared execution contracts so control-plane commands are predictable for
agents and scripts before individual service commands are implemented.

## User-facing Commands

Applies globally to:

- `tdc db <subcommand>`
- `tdc fs <control-plane-subcommand>`
- `tdc organization <subcommand>`

## Behavior

- Successful control-plane commands output JSON by default.
- Support `--output json` and `--output human`.
- Support `--query <jmespath-expression>` on structured command results.
- Apply `--query` after command execution and before rendering.
- Mutating control-plane commands support `--dry-run`.
- `--dry-run` validates inputs, config, credentials, endpoint selection, and
  request construction without creating, updating, or deleting remote resources.
- Data-plane fs commands may stream file bytes or command-shaped text where JSON
  would break normal filesystem usage.

## Inputs And Config

- `--output` defaults to `json` for control-plane commands.
- `--query` requires a structured result. Commands that stream raw bytes should
  reject `--query` with an actionable error.
- `--dry-run` is valid only on mutating control-plane commands.

## Output And Errors

- JSON output must be stable and deterministic.
- Human output must be concise and must not be the only output mode for
  automation-critical data.
- All errors render as:

```text
tdc [ERROR]: <actionable message>
```

- Error messages should include the next action when the failure is user
  recoverable.
- Authentication and authorization errors must preserve their category so
  scripts can distinguish missing credentials, invalid credentials, and
  permission denied.

## After This Spec

Every later control-plane command automatically gets agent-friendly output
controls:

```bash
tdc db list-db-clusters
tdc db list-db-clusters --query 'clusters[0].id'
tdc db create-db-cluster --db-cluster-name demo --db-cluster-type starter --dry-run
tdc organization list-projects --output human
```

This step adds shared rendering and validation behavior. It does not create any
service-specific API response shapes.

## Implementation Design

- `internal/output` renders JSON, human output adapters, and raw stream
  pass-through for data-plane commands.
- `internal/query` applies JMESPath expressions to structured Go values before
  rendering.
- `internal/apperr` maps validation, config, authentication, authorization,
  not-found, conflict, rate-limit, and unknown errors to stable codes and exit
  codes.
- `internal/dryrun` defines a common result envelope for mutating control-plane
  commands, including the validated request summary and
  `would_send_request: true`.
- Command handlers return structured results plus metadata; they do not render
  directly except for explicitly raw data-plane streams.

## API Call Chain

No service-specific remote API is introduced by this spec. `--dry-run` stops
before mutating API calls; each later command spec defines which request is
constructed and reported.

## Dependencies And Platform

- Add `github.com/jmespath/go-jmespath` for `--query`.
- JSON rendering uses the Go standard library.
- No cgo is required.
- Query and rendering packages must be platform-neutral.

## Dependencies

- `0001-cli-foundation.md`.
- `0002-local-config-and-credentials.md`.

## Acceptance Criteria

- Tests cover JSON default rendering.
- Tests cover `--output human`.
- Tests cover `--query` success and invalid query failure.
- Tests cover `--dry-run` on mutating commands.
- Tests cover rejection of `--dry-run` on read-only commands.
- Tests cover the standard error prefix.

## Out Of Scope

- Defining every service-specific response field.
- Supporting output formats beyond JSON and human.
