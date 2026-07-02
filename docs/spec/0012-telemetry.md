# Telemetry

## Goal

Collect minimal, privacy-preserving CLI telemetry that helps improve tdc without
capturing sensitive user data.

## User-facing Commands

Telemetry applies to all commands. Any explicit enable/disable command or config
knob must remain within the two-level command rule if added.

## Behavior

- Track command and subcommand invoked.
- Track flag names used, never flag values.
- Track error codes and execution time.
- Track TiDB Cloud region, CLI version, and OS type.
- Do not block command completion on telemetry delivery.
- Telemetry failures must not fail the user command.
- Disable telemetry in development builds by default.

## Inputs And Config

Telemetry may read:

- resolved profile name
- region code
- command path
- flag names
- version and OS metadata

Telemetry must not read or send:

- public or private API keys
- `tdc fs` resource API keys
- generated DB SQL usernames and passwords
- fs file contents
- SQL text
- local file contents
- raw API payloads
- flag values

## Output And Errors

- Telemetry does not produce normal user output.
- In debug mode, telemetry logs must redact all values and avoid secrets.

## After This Spec

All implemented commands emit privacy-preserving telemetry in release builds
without changing user workflows:

```bash
tdc db list-db-clusters
tdc fs check-file-system --debug
tdc organization list-projects --query 'projects[0].id'
```

Telemetry helps identify failing command categories and region/provider usage
without recording command results or secrets.

## Implementation Design

- `internal/telemetry` owns event models, allowlists, redaction, async delivery,
  and no-op behavior.
- `internal/cli` starts a telemetry span after parsing the command path and flag
  names.
- `internal/apperr` exposes stable error codes for telemetry without exposing
  raw messages.
- `internal/config` provides profile name, cloud provider, and region metadata
  only after redaction rules are applied.
- Delivery must be best-effort and cancellable; command completion must not wait
  on slow telemetry network calls.

## API Call Chain

No telemetry ingestion endpoint is confirmed by the current project refs.

MVP behavior:

- Development builds use a no-op telemetry sink by default.
- Release builds may enable telemetry only after a product-owned HTTPS ingestion
  endpoint, auth mode, retention policy, and opt-out behavior are confirmed.
- Until that endpoint exists, telemetry implementation should stop at event
  construction, redaction tests, and a disabled sender.

## Dependencies And Platform

- No third-party dependency is required for the MVP telemetry client.
- Use standard library `net/http`, `runtime`, `time`, and `context`.
- No cgo is required.
- Platform-neutral.

## Dependencies

- `0001-cli-foundation.md`.
- `0002-local-config-and-credentials.md`.
- `0003-output-error-query-dry-run.md`.

## Acceptance Criteria

- Tests verify captured event fields contain flag names but not values.
- Tests verify credentials are redacted or absent.
- Tests verify telemetry send failures do not alter command exit status.
- Tests verify development builds do not send telemetry by default.

## Out Of Scope

- Product analytics dashboards.
- Capturing command output or API response bodies.
