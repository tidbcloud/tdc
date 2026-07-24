package telemetrybackend

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeSQLDatabase struct {
	mu      sync.Mutex
	queries []string
	args    [][]any
	pingErr error
	execErr error
}

func (f *fakeSQLDatabase) PingContext(context.Context) error {
	return f.pingErr
}

func (f *fakeSQLDatabase) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, query)
	f.args = append(f.args, append([]any(nil), args...))
	return fakeSQLResult(1), f.execErr
}

type fakeSQLResult int64

func (r fakeSQLResult) LastInsertId() (int64, error) { return 0, nil }
func (r fakeSQLResult) RowsAffected() (int64, error) { return int64(r), nil }

func TestTiDBSinkCreatesSchemaAndBatchInsertsSanitizedEvents(t *testing.T) {
	db := &fakeSQLDatabase{}
	sink := NewTiDBSink(db)
	if err := sink.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema returned error: %v", err)
	}
	events := []Event{testEvent(), testEvent()}
	events[1].EventID = "018f7e67-8fe4-7cc2-9ca5-2d3536c7fb45"
	if err := sink.Write(context.Background(), events); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if len(db.queries) != 2 {
		t.Fatalf("query count = %d, want 2", len(db.queries))
	}
	if !strings.Contains(db.queries[0], "CREATE TABLE IF NOT EXISTS telemetry_events") {
		t.Fatalf("schema query = %s", db.queries[0])
	}
	if !strings.HasPrefix(db.queries[1], "INSERT IGNORE INTO telemetry_events") {
		t.Fatalf("insert query = %s", db.queries[1])
	}
	if len(db.args[1]) != 36 {
		t.Fatalf("insert args = %d, want 36", len(db.args[1]))
	}
	if flags, ok := db.args[1][6].(string); !ok || flags != `["file-system-name","output"]` {
		t.Fatalf("flag_names_json = %#v", db.args[1][6])
	}
	for _, query := range db.queries {
		if strings.Contains(query, "SELECT") || strings.Contains(query, "password") {
			t.Fatalf("unexpected sensitive query text: %s", query)
		}
	}
}

func TestTiDBSinkIntegration(t *testing.T) {
	dsn := os.Getenv("TDC_TEST_TELEMETRY_TIDB_DSN")
	if dsn == "" {
		t.Skip("TDC_TEST_TELEMETRY_TIDB_DSN is not set")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sink := NewTiDBSink(db)
	if err := sink.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema returned error: %v", err)
	}
	event := testEvent()
	event.EventID = "tdc-telemetry-integration-event"
	_, _ = db.ExecContext(ctx, "DELETE FROM telemetry_events WHERE event_id = ?", event.EventID)
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupContext, "DELETE FROM telemetry_events WHERE event_id = ?", event.EventID)
	})
	if err := sink.Write(ctx, []Event{event}); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	var commandPath string
	var flagNames string
	if err := db.QueryRowContext(
		ctx,
		"SELECT command_path, flag_names_json FROM telemetry_events WHERE event_id = ?",
		event.EventID,
	).Scan(&commandPath, &flagNames); err != nil {
		t.Fatalf("query inserted event: %v", err)
	}
	var storedFlagNames []string
	if err := json.Unmarshal([]byte(flagNames), &storedFlagNames); err != nil {
		t.Fatalf("decode stored flag_names_json: %v", err)
	}
	if commandPath != event.CommandPath ||
		len(storedFlagNames) != 2 ||
		storedFlagNames[0] != "file-system-name" ||
		storedFlagNames[1] != "output" {
		t.Fatalf("stored event = command %q, flags %q", commandPath, flagNames)
	}
}
