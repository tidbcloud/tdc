package telemetrybackend

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDecodeAndValidateBatchAcceptsAllowlistedEvent(t *testing.T) {
	receivedAt := time.Now()
	events, err := decodeAndValidateBatch(validRequestBody(), 20, receivedAt)
	if err != nil {
		t.Fatalf("decodeAndValidateBatch returned error: %v", err)
	}
	if len(events) != 1 || events[0].EventName != "tdc.command.finished" {
		t.Fatalf("events = %#v", events)
	}
	if !events[0].ReceivedAt.Equal(receivedAt.UTC()) {
		t.Fatalf("ReceivedAt = %v, want %v", events[0].ReceivedAt, receivedAt.UTC())
	}
}

func TestDecodeAndValidateBatchRejectsUnknownAndProhibitedFields(t *testing.T) {
	base := validRequestBody()
	var request map[string]any
	if err := json.Unmarshal(base, &request); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "unknown top level",
			mutate: func(value map[string]any) {
				value["extra"] = "value"
			},
		},
		{
			name: "prohibited event field",
			mutate: func(value map[string]any) {
				value["events"].([]any)[0].(map[string]any)["sql"] = "select secret"
			},
		},
		{
			name: "nested prohibited field",
			mutate: func(value map[string]any) {
				value["extra"] = map[string]any{"password": "secret"}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var copyValue map[string]any
			encoded, _ := json.Marshal(request)
			_ = json.Unmarshal(encoded, &copyValue)
			test.mutate(copyValue)
			body, _ := json.Marshal(copyValue)
			if _, err := decodeAndValidateBatch(body, 20, time.Now()); err == nil {
				t.Fatal("decodeAndValidateBatch accepted invalid body")
			}
		})
	}
}

func TestDecodeAndValidateBatchRejectsMissingRequiredNumbers(t *testing.T) {
	for _, field := range []string{"exit_code", "duration_ms"} {
		t.Run(field, func(t *testing.T) {
			var request map[string]any
			_ = json.Unmarshal(validRequestBody(), &request)
			delete(request["events"].([]any)[0].(map[string]any), field)
			body, _ := json.Marshal(request)
			if _, err := decodeAndValidateBatch(body, 20, time.Now()); err == nil {
				t.Fatalf("missing %s was accepted", field)
			}
		})
	}
}

func TestDecodeAndValidateBatchRejectsInvalidEnumsAndLimits(t *testing.T) {
	tests := []struct {
		field string
		value any
	}{
		{"event_name", "custom.event"},
		{"command_path", "rm -rf"},
		{"command_path", "tdc db list-db-clusters select secret"},
		{"cloud_provider", "gcp"},
		{"region_code", "us-east-1"},
		{"install_source", "curl"},
		{"profile_source", "production"},
		{"os", "my-secret-host"},
		{"arch", "custom-cpu"},
		{"cli_version", "version with spaces"},
		{"exit_code", 256},
		{"duration_ms", 86_400_001},
	}
	for _, test := range tests {
		t.Run(test.field, func(t *testing.T) {
			var request map[string]any
			_ = json.Unmarshal(validRequestBody(), &request)
			request["events"].([]any)[0].(map[string]any)[test.field] = test.value
			body, _ := json.Marshal(request)
			if _, err := decodeAndValidateBatch(body, 20, time.Now()); err == nil {
				t.Fatalf("%s=%v was accepted", test.field, test.value)
			}
		})
	}
}

func TestDecodeAndValidateBatchRejectsTooManyEventsAndTrailingJSON(t *testing.T) {
	var request map[string]any
	_ = json.Unmarshal(validRequestBody(), &request)
	event := request["events"].([]any)[0]
	request["events"] = []any{event, event}
	body, _ := json.Marshal(request)
	if _, err := decodeAndValidateBatch(body, 1, time.Now()); err == nil {
		t.Fatal("too many events were accepted")
	}

	body = append(validRequestBody(), []byte(` {}`)...)
	if _, err := decodeAndValidateBatch(body, 20, time.Now()); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
}

func TestRejectedBodyDoesNotNeedToExposeValues(t *testing.T) {
	body := bytes.Replace(validRequestBody(), []byte(`"events":[`), []byte(`"sql":"highly-sensitive","events":[`), 1)
	_, err := decodeAndValidateBatch(body, 20, time.Now())
	if err == nil {
		t.Fatal("prohibited field was accepted")
	}
	if strings.Contains(err.Error(), "highly-sensitive") {
		t.Fatalf("error exposed rejected value: %v", err)
	}
}
