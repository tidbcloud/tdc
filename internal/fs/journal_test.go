package fs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apifs "github.com/Icemap/tdc/internal/api/fs"
)

func TestCreateJournalSendsNormalizedLabelsAndActor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/journals" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		var req apifs.JournalCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.JournalID != "jrn_test" || req.Kind != "agent" || req.Title != "live task" {
			t.Fatalf("unexpected request: %#v", req)
		}
		if req.Actor.Type != "agent" || req.Actor.ID != "tdc" {
			t.Fatalf("actor = %#v", req.Actor)
		}
		if len(req.Labels) != 2 || req.Labels[0].Key != "env" || req.Labels[0].Value != "prod" || req.Labels[1].Key != "env" || req.Labels[1].Value != "us-east" {
			t.Fatalf("labels = %#v", req.Labels)
		}
		_ = json.NewEncoder(w).Encode(apifs.Journal{JournalID: "jrn_test", Kind: "agent", Title: "live task"})
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).CreateJournal(context.Background(), JournalCreateOptions{
		Profile:     dataProfile(),
		JournalID:   "jrn_test",
		JournalKind: "agent",
		Title:       "live task",
		Actor:       "agent:tdc",
		Labels:      []string{"Env=us-east", "env=prod"},
	})
	if err != nil {
		t.Fatalf("CreateJournal failed: %v", err)
	}
	if apifs.Journal(result).JournalID != "jrn_test" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAppendJournalEntriesParsesEntryJSONAndDefaults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/journals/jrn_test/entries" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		if got := r.Header.Get("Idempotency-Key"); got != "app_test" {
			t.Fatalf("Idempotency-Key = %q", got)
		}
		var entries []apifs.JournalEntryInput
		if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(entries) != 1 || entries[0].Type != "task.started" || entries[0].Source != "self_reported" {
			t.Fatalf("entries = %#v", entries)
		}
		if len(entries[0].Subjects) != 2 || entries[0].Subjects[0] != "repo:tdc" || entries[0].Subjects[1] != "task:1" {
			t.Fatalf("subjects = %#v", entries[0].Subjects)
		}
		_ = json.NewEncoder(w).Encode(apifs.JournalAppendResponse{JournalID: "jrn_test", AppendID: "app_test", FirstSeq: 1, LastSeq: 1, Count: 1})
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).AppendJournalEntries(context.Background(), JournalAppendOptions{
		Profile:        dataProfile(),
		JournalID:      "jrn_test",
		IdempotencyKey: "app_test",
		EntryType:      "task.started",
		Source:         "self_reported",
		Subjects:       []string{"repo:tdc"},
		EntryJSON:      []string{`{"subjects":["task:1"],"summary":{"message":"hello"}}`},
	})
	if err != nil {
		t.Fatalf("AppendJournalEntries failed: %v", err)
	}
	if apifs.JournalAppendResponse(result).Count != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAppendJournalEntriesReadsJSONLFromStdin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var entries []apifs.JournalEntryInput
		if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(entries) != 2 || entries[0].Type != "task.started" || entries[1].Type != "task.completed" {
			t.Fatalf("entries = %#v", entries)
		}
		_ = json.NewEncoder(w).Encode(apifs.JournalAppendResponse{JournalID: "jrn_test", AppendID: "app_test", Count: 2})
	}))
	defer server.Close()

	_, err := testService(t.TempDir(), server.URL).AppendJournalEntries(context.Background(), JournalAppendOptions{
		Profile:        dataProfile(),
		JournalID:      "jrn_test",
		IdempotencyKey: "app_test",
		Stdin:          strings.NewReader(`{"type":"task.started"}` + "\n" + `{"type":"task.completed"}` + "\n"),
	})
	if err != nil {
		t.Fatalf("AppendJournalEntries failed: %v", err)
	}
}

func TestSearchJournalPassesFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/journal-entries" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
		if r.URL.Query().Get("actor") != "agent:tdc" || r.URL.Query().Get("include") != "entry" || r.URL.Query().Get("since") != "1h" {
			t.Fatalf("query = %q", r.URL.RawQuery)
		}
		meta := r.URL.Query()["meta"]
		if len(meta) != 1 || meta[0] != "env=prod" {
			t.Fatalf("meta = %#v", meta)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"journal_id":"jrn_test","seq":1,"type":"task.started","cursor":"cur"}` + "\n"))
	}))
	defer server.Close()

	result, err := testService(t.TempDir(), server.URL).SearchJournal(context.Background(), JournalSearchOptions{
		Profile:        dataProfile(),
		EntryType:      "task.started",
		Actor:          "agent:tdc",
		Labels:         []string{"env=prod"},
		Since:          "1h",
		IncludeEntries: true,
	})
	if err != nil {
		t.Fatalf("SearchJournal failed: %v", err)
	}
	if len(result.Matches) != 1 || result.Matches[0].Cursor != "cur" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAppendJournalEntriesRequiresType(t *testing.T) {
	_, err := testService(t.TempDir(), "https://fs.test").AppendJournalEntries(context.Background(), JournalAppendOptions{
		Profile:   dataProfile(),
		JournalID: "jrn_test",
		EntryJSON: []string{`{"summary":{"message":"hello"}}`},
	})
	if err == nil || !strings.Contains(err.Error(), "missing required type") {
		t.Fatalf("expected missing type error, got %v", err)
	}
}
