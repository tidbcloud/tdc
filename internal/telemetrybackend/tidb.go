package telemetrybackend

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

const createTelemetryTableSQL = `CREATE TABLE IF NOT EXISTS telemetry_events (
  event_id VARCHAR(64) NOT NULL,
  received_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  occurred_at TIMESTAMP(6) NOT NULL,
  anonymous_installation_id VARCHAR(128) NOT NULL,
  event_name VARCHAR(64) NOT NULL,
  command_path VARCHAR(128) NOT NULL,
  flag_names_json JSON NOT NULL,
  exit_code TINYINT UNSIGNED NOT NULL,
  error_code VARCHAR(64) NOT NULL DEFAULT '',
  duration_ms INT UNSIGNED NOT NULL,
  cloud_provider VARCHAR(32) NOT NULL DEFAULT '',
  region_code VARCHAR(64) NOT NULL DEFAULT '',
  cli_version VARCHAR(64) NOT NULL,
  os VARCHAR(32) NOT NULL,
  arch VARCHAR(32) NOT NULL,
  install_source VARCHAR(32) NOT NULL DEFAULT '',
  profile_source VARCHAR(32) NOT NULL DEFAULT '',
  schema_version INT UNSIGNED NOT NULL,
  PRIMARY KEY (event_id),
  KEY idx_received_at (received_at),
  KEY idx_command_received (command_path, received_at),
  KEY idx_version_received (cli_version, received_at),
  KEY idx_region_received (cloud_provider, region_code, received_at)
)`

type sqlDatabase interface {
	PingContext(context.Context) error
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type TiDBSink struct {
	db sqlDatabase
}

func NewTiDBSink(db sqlDatabase) *TiDBSink {
	return &TiDBSink{db: db}
}

func (s *TiDBSink) Name() string {
	return "tidb"
}

func (s *TiDBSink) Ready(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *TiDBSink) EnsureSchema(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, createTelemetryTableSQL); err != nil {
		return fmt.Errorf("create telemetry table: %w", err)
	}
	return nil
}

func (s *TiDBSink) Write(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	const columns = `(event_id, received_at, occurred_at, anonymous_installation_id,
event_name, command_path, flag_names_json, exit_code, error_code, duration_ms,
cloud_provider, region_code, cli_version, os, arch, install_source,
profile_source, schema_version)`
	const placeholders = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

	values := make([]string, 0, len(events))
	args := make([]any, 0, len(events)*18)
	for _, event := range events {
		flagNames, err := json.Marshal(event.FlagNames)
		if err != nil {
			return fmt.Errorf("encode flag names: %w", err)
		}
		values = append(values, placeholders)
		args = append(args,
			event.EventID,
			event.ReceivedAt,
			event.OccurredAt,
			event.AnonymousInstallationID,
			event.EventName,
			event.CommandPath,
			string(flagNames),
			event.ExitCode,
			event.ErrorCode,
			event.DurationMS,
			event.CloudProvider,
			event.RegionCode,
			event.CLIVersion,
			event.OS,
			event.Arch,
			event.InstallSource,
			event.ProfileSource,
			event.SchemaVersion,
		)
	}
	query := "INSERT IGNORE INTO telemetry_events " + columns + " VALUES " + strings.Join(values, ",")
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("insert telemetry batch: %w", err)
	}
	return nil
}
