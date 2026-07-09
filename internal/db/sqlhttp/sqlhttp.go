package sqlhttp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/db/sqlcred"
	"github.com/tidbcloud/tdc/internal/db/sqlresult"
)

type Options struct {
	ClusterID   string
	AccessMode  sqlcred.AccessMode
	Username    string
	Password    string
	Host        string
	Database    string
	SQL         string
	BaseURL     string
	HTTPClient  *http.Client
	Debug       bool
	DebugWriter io.Writer
	UserAgent   string
}

func Execute(ctx context.Context, opts Options) (sqlresult.Result, error) {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	endpoint, err := endpointURL(opts)
	if err != nil {
		return sqlresult.Result{}, err
	}
	body, err := json.Marshal(map[string]string{"query": opts.SQL})
	if err != nil {
		return sqlresult.Result{}, apperr.Wrap("db.sql_http_encode", "runtime", 1, "encode SQL HTTP request", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return sqlresult.Result{}, apperr.Wrap("db.sql_http_request", "runtime", 1, "build SQL HTTP request", err)
	}
	req.SetBasicAuth(opts.Username, opts.Password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("TiDB-Database", opts.Database)
	req.Header.Set("TiDB-Session", "")
	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	} else {
		req.Header.Set("User-Agent", "tdc")
	}
	traceID := traceID()
	req.Header.Set("X-Debug-Trace-Id", traceID)
	if opts.Debug && opts.DebugWriter != nil {
		_, _ = fmt.Fprintf(opts.DebugWriter, "tdc [DEBUG]: sql http request id: %s\n", traceID)
	}

	res, err := client.Do(req)
	if err != nil {
		return sqlresult.Result{}, apperr.Wrap("db.sql_http_network", "database", 1, "SQL HTTP request failed: check network connectivity and cluster endpoint", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return sqlresult.Result{}, statusError(res)
	}
	var wire responseWire
	if err := json.NewDecoder(res.Body).Decode(&wire); err != nil {
		return sqlresult.Result{}, apperr.Wrap("db.sql_http_decode", "database", 1, "SQL HTTP response was not valid JSON", err)
	}
	fields := wire.Fields()
	rows := sqlresult.DecodeRows(fields, wire.Rows)
	return sqlresult.Result{
		Fields:       fields,
		Rows:         rows,
		RowCount:     len(rows),
		RowsAffected: wire.RowsAffected,
		LastInsertID: wire.LastInsertID,
		Transport:    "http",
		AccessMode:   opts.AccessMode,
		ClusterID:    opts.ClusterID,
		Session:      res.Header.Get("TiDB-Session"),
	}, nil
}

func endpointURL(opts Options) (string, error) {
	if opts.BaseURL != "" {
		parsed, err := url.Parse(opts.BaseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return "", apperr.Wrap("db.sql_http_invalid_endpoint", "config", 2, fmt.Sprintf("invalid SQL HTTP endpoint %q", opts.BaseURL), err)
		}
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/v1beta/sql"
		return parsed.String(), nil
	}
	if strings.TrimSpace(opts.Host) == "" {
		return "", apperr.New("db.sql_http_missing_host", "api", 1, "cluster public endpoint host is missing; describe the cluster and try again after it becomes ACTIVE")
	}
	return "https://http-" + opts.Host + "/v1beta/sql", nil
}

func statusError(res *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
	message := ""
	var payload struct {
		Message string `json:"message"`
		Error   string `json:"error"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		message = payload.Message
		if message == "" {
			message = payload.Error
		}
	}
	if message == "" {
		message = fmt.Sprintf("SQL HTTP request failed with HTTP %d", res.StatusCode)
	}
	return apperr.New("db.sql_execution_failed", "database", 1, message)
}

type responseWire struct {
	Types        []fieldWire `json:"types"`
	Rows         [][]*string `json:"rows"`
	RowsAffected *int64      `json:"rowsAffected"`
	LastInsertID *string     `json:"sLastInsertID"`
}

func (r responseWire) Fields() []sqlresult.Field {
	fields := make([]sqlresult.Field, 0, len(r.Types))
	for _, field := range r.Types {
		fields = append(fields, sqlresult.Field{
			Name:     field.Name,
			Type:     field.Type,
			Nullable: field.Nullable,
		})
	}
	return fields
}

type fieldWire struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

func traceID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(data[:])
}
