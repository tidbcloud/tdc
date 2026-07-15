package fs

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	apifs "github.com/tidbcloud/tdc/internal/api/fs"
	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/authz"
	"github.com/tidbcloud/tdc/internal/config"
)

type JournalCreateOptions struct {
	Profile     *config.Profile
	JournalID   string
	JournalKind string
	Title       string
	Actor       string
	Labels      []string
}

type JournalAppendOptions struct {
	Profile        *config.Profile
	JournalID      string
	IdempotencyKey string
	EntryType      string
	Source         string
	Subjects       []string
	EntryJSON      []string
	JSONArray      bool
	Stdin          io.Reader
}

type JournalReadOptions struct {
	Profile   *config.Profile
	JournalID string
	AfterSeq  int64
	Limit     int
}

type JournalSearchOptions struct {
	Profile        *config.Profile
	EntryType      string
	Status         string
	JournalKind    string
	Actor          string
	Subjects       []string
	Labels         []string
	Since          string
	Until          string
	Limit          int
	Cursor         string
	IncludeEntries bool
}

type JournalVerifyOptions struct {
	Profile   *config.Profile
	JournalID string
}

type JournalResult apifs.Journal

type JournalAppendResult apifs.JournalAppendResponse

type JournalEntriesResult struct {
	Entries []apifs.JournalEntry `json:"entries"`
}

type JournalSearchResult struct {
	Matches []apifs.JournalSearchMatch `json:"matches"`
}

type JournalVerifyResult apifs.JournalVerifyResult

func (s Service) CreateJournal(ctx context.Context, opts JournalCreateOptions) (JournalResult, error) {
	if s.UseDrive9Companion {
		return s.drive9CreateJournal(ctx, opts)
	}
	client, err := s.dataClient(opts.Profile, authz.FSJournalCreate, "create tdc fs-journal")
	if err != nil {
		return JournalResult{}, err
	}
	journalID := strings.TrimSpace(opts.JournalID)
	if journalID == "" {
		journalID = newJournalID("jrn")
	}
	kind := strings.TrimSpace(opts.JournalKind)
	if kind == "" {
		kind = apifs.DefaultJournalKind
	}
	labels, err := parseJournalLabels(opts.Labels)
	if err != nil {
		return JournalResult{}, err
	}
	actorType, actorID, err := apifs.SplitJournalActor(opts.Actor)
	if err != nil {
		return JournalResult{}, err
	}
	journal, err := client.CreateJournal(ctx, apifs.JournalCreateRequest{
		JournalID: journalID,
		Kind:      kind,
		Title:     strings.TrimSpace(opts.Title),
		Actor:     apifs.JournalActor{Type: actorType, ID: actorID},
		Labels:    labels,
	})
	if err != nil {
		return JournalResult{}, err
	}
	return JournalResult(journal), nil
}

func (s Service) AppendJournalEntries(ctx context.Context, opts JournalAppendOptions) (JournalAppendResult, error) {
	if s.UseDrive9Companion {
		return s.drive9AppendJournalEntries(ctx, opts)
	}
	client, err := s.dataClient(opts.Profile, authz.FSJournalAppend, "append tdc fs-journal entries")
	if err != nil {
		return JournalAppendResult{}, err
	}
	journalID, err := requireJournalID(opts.JournalID)
	if err != nil {
		return JournalAppendResult{}, err
	}
	appendID := strings.TrimSpace(opts.IdempotencyKey)
	if appendID == "" {
		appendID = newJournalID("app")
	}
	entries, err := parseJournalEntryInputs(opts.EntryJSON, opts.Stdin, opts.JSONArray)
	if err != nil {
		return JournalAppendResult{}, err
	}
	defaultType := strings.TrimSpace(opts.EntryType)
	source := strings.TrimSpace(opts.Source)
	for i := range entries {
		if entries[i].Type == "" {
			entries[i].Type = defaultType
		}
		if entries[i].Type == "" {
			return JournalAppendResult{}, apperr.New("journal.missing_entry_type", "usage", 2, fmt.Sprintf("journal entry %d missing required type; provide --entry-type or include type in the entry JSON", i+1))
		}
		if source != "" {
			entries[i].Source = source
		}
		if len(opts.Subjects) > 0 {
			entries[i].Subjects = append(append([]string{}, opts.Subjects...), entries[i].Subjects...)
		}
	}
	response, err := client.AppendJournalEntries(ctx, journalID, appendID, entries)
	if err != nil {
		return JournalAppendResult{}, err
	}
	return JournalAppendResult(response), nil
}

func (s Service) ReadJournalEntries(ctx context.Context, opts JournalReadOptions) (JournalEntriesResult, error) {
	if s.UseDrive9Companion {
		return s.drive9ReadJournalEntries(ctx, opts)
	}
	client, err := s.dataClient(opts.Profile, authz.FSJournalRead, "read tdc fs-journal entries")
	if err != nil {
		return JournalEntriesResult{}, err
	}
	journalID, err := requireJournalID(opts.JournalID)
	if err != nil {
		return JournalEntriesResult{}, err
	}
	limit := normalizeJournalLimit(opts.Limit)
	entries, err := client.ReadJournalEntries(ctx, journalID, opts.AfterSeq, limit)
	if err != nil {
		return JournalEntriesResult{}, err
	}
	return JournalEntriesResult{Entries: entries}, nil
}

func (s Service) SearchJournal(ctx context.Context, opts JournalSearchOptions) (JournalSearchResult, error) {
	if s.UseDrive9Companion {
		return s.drive9SearchJournal(ctx, opts)
	}
	client, err := s.dataClient(opts.Profile, authz.FSJournalSearch, "search tdc fs-journal")
	if err != nil {
		return JournalSearchResult{}, err
	}
	labels, err := parseJournalLabels(opts.Labels)
	if err != nil {
		return JournalSearchResult{}, err
	}
	actorType, actorID, err := apifs.SplitJournalActor(opts.Actor)
	if err != nil {
		return JournalSearchResult{}, err
	}
	matches, err := client.SearchJournal(ctx, apifs.JournalSearchRequest{
		Type:      strings.TrimSpace(opts.EntryType),
		Status:    strings.TrimSpace(opts.Status),
		Kind:      strings.TrimSpace(opts.JournalKind),
		ActorType: actorType,
		ActorID:   actorID,
		Subjects:  opts.Subjects,
		Labels:    labels,
		SinceRaw:  strings.TrimSpace(opts.Since),
		UntilRaw:  strings.TrimSpace(opts.Until),
		Limit:     normalizeJournalLimit(opts.Limit),
		Entries:   opts.IncludeEntries,
		Cursor:    strings.TrimSpace(opts.Cursor),
	})
	if err != nil {
		return JournalSearchResult{}, err
	}
	return JournalSearchResult{Matches: matches}, nil
}

func (s Service) VerifyJournal(ctx context.Context, opts JournalVerifyOptions) (JournalVerifyResult, error) {
	if s.UseDrive9Companion {
		return s.drive9VerifyJournal(ctx, opts)
	}
	client, err := s.dataClient(opts.Profile, authz.FSJournalVerify, "verify tdc fs-journal")
	if err != nil {
		return JournalVerifyResult{}, err
	}
	journalID, err := requireJournalID(opts.JournalID)
	if err != nil {
		return JournalVerifyResult{}, err
	}
	result, err := client.VerifyJournal(ctx, journalID)
	if err != nil {
		return JournalVerifyResult{}, err
	}
	return JournalVerifyResult(result), nil
}

func requireJournalID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", apperr.New("journal.missing_journal_id", "usage", 2, "--journal-id is required")
	}
	return id, nil
}

func normalizeJournalLimit(limit int) int {
	if limit <= 0 {
		return apifs.DefaultJournalLimit
	}
	if limit > apifs.MaxJournalLimit {
		return apifs.MaxJournalLimit
	}
	return limit
}

func parseJournalLabels(values []string) ([]apifs.JournalLabel, error) {
	if len(values) == 0 {
		return nil, nil
	}
	labels := make([]apifs.JournalLabel, 0, len(values))
	for _, raw := range values {
		key, value, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, apperr.New("journal.invalid_label", "usage", 2, fmt.Sprintf("invalid label %q; expected key=value", raw))
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, apperr.New("journal.invalid_label", "usage", 2, fmt.Sprintf("invalid label %q; key is empty", raw))
		}
		labels = append(labels, apifs.JournalLabel{Key: key, Value: strings.TrimSpace(value)})
	}
	return sortedJournalLabels(apifs.NormalizeJournalLabels(labels)), nil
}

func parseJournalEntryInputs(entryJSON []string, stdin io.Reader, jsonArray bool) ([]apifs.JournalEntryInput, error) {
	if len(entryJSON) > 0 {
		entries := make([]apifs.JournalEntryInput, 0, len(entryJSON))
		for i, raw := range entryJSON {
			var entry apifs.JournalEntryInput
			if err := json.Unmarshal([]byte(raw), &entry); err != nil {
				return nil, apperr.Wrap("journal.decode_entry_json", "usage", 2, fmt.Sprintf("decode --entry-json %d", i+1), err)
			}
			entries = append(entries, entry)
		}
		return entries, nil
	}
	if stdin == nil {
		return nil, apperr.New("journal.missing_entries", "usage", 2, "provide --entry-json or pipe JSONL entries on stdin")
	}
	if jsonArray {
		var entries []apifs.JournalEntryInput
		if err := json.NewDecoder(stdin).Decode(&entries); err != nil {
			return nil, apperr.Wrap("journal.decode_json_array", "usage", 2, "decode journal JSON array from stdin", err)
		}
		if len(entries) == 0 {
			return nil, apperr.New("journal.missing_entries", "usage", 2, "no journal entries on stdin")
		}
		return entries, nil
	}
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	var entries []apifs.JournalEntryInput
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry apifs.JournalEntryInput
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, apperr.Wrap("journal.decode_jsonl", "usage", 2, fmt.Sprintf("decode journal JSONL at line %d", lineNum), err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, apperr.Wrap("journal.read_stdin", "runtime", 1, "read journal entries from stdin", err)
	}
	if len(entries) == 0 {
		return nil, apperr.New("journal.missing_entries", "usage", 2, "no journal entries on stdin")
	}
	return entries, nil
}

func newJournalID(prefix string) string {
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		return prefix + "_" + time.Now().UTC().Format("20060102150405")
	}
	return prefix + "_" + time.Now().UTC().Format("20060102150405") + "_" + hex.EncodeToString(random[:])
}

func (j JournalResult) Human() string {
	journal := apifs.Journal(j)
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tKIND\tTITLE\tNEXT_SEQ\tHEAD_HASH")
	_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", journal.JournalID, journal.Kind, journal.Title, journal.NextSeq, journal.HeadHash)
	_ = w.Flush()
	return b.String()
}

func (r JournalEntriesResult) Human() string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "SEQ\tTYPE\tSTATUS\tOBSERVED_AT\tENTRY_ID")
	for _, entry := range r.Entries {
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", entry.Seq, entry.Type, entry.Status, formatJournalTime(entry.ObservedAt), entry.EntryID)
	}
	_ = w.Flush()
	return b.String()
}

func (r JournalSearchResult) Human() string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "JOURNAL_ID\tSEQ\tTYPE\tKIND\tTIME\tCURSOR")
	for _, match := range r.Matches {
		timestamp := match.ObservedAt
		if timestamp.IsZero() {
			timestamp = match.CreatedAt
		}
		_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\n", match.JournalID, match.Seq, match.Type, match.Kind, formatJournalTime(timestamp), match.Cursor)
	}
	_ = w.Flush()
	return b.String()
}

func (r JournalAppendResult) Human() string {
	response := apifs.JournalAppendResponse(r)
	return fmt.Sprintf("journal=%s append=%s seq=%d-%d count=%d head=%s idempotent=%t\n", response.JournalID, response.AppendID, response.FirstSeq, response.LastSeq, response.Count, response.HeadHash, response.Idempotent)
}

func (r JournalVerifyResult) Human() string {
	result := apifs.JournalVerifyResult(r)
	status := "failed"
	if result.OK {
		status = "ok"
	}
	return fmt.Sprintf("%s journal=%s entries=%d head=%s\n", status, result.JournalID, result.Entries, result.HeadHash)
}

func formatJournalTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func sortedJournalLabels(labels []apifs.JournalLabel) []apifs.JournalLabel {
	out := append([]apifs.JournalLabel(nil), labels...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Key == out[j].Key {
			return out[i].Value < out[j].Value
		}
		return out[i].Key < out[j].Key
	})
	return out
}
