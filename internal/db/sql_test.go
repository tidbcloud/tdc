package db

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidbcloud/tdc/internal/api/endpoints"
	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/db/connectionstring"
	"github.com/tidbcloud/tdc/internal/db/sqlcred"
)

func TestPrepareQueryAccessCreatesAndStoresCredentials(t *testing.T) {
	var created []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1beta1/clusters/cluster-1":
			_, _ = w.Write([]byte(`{"clusterId":"cluster-1","displayName":"demo","clusterPlan":"STARTER","endpoints":{"public":{"host":"gateway.example.com","port":4000}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1beta1/clusters/cluster-1/sqlUsers":
			_, _ = w.Write([]byte(`{"sqlUsers":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1beta1/clusters/cluster-1/sqlUsers":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			created = append(created, body)
			_, _ = w.Write([]byte(`{"userName":"prefix.` + body["userName"].(string) + `","authMethod":"mysql_native_password","builtinRole":"` + body["builtinRole"].(string) + `"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	home := t.TempDir()
	result, err := testSQLService(server.URL, home).PrepareQueryAccess(context.Background(), PrepareQueryAccessOptions{
		Profile:   testProfile(),
		ClusterID: "cluster-1",
	})
	if err != nil {
		t.Fatalf("PrepareQueryAccess failed: %v", err)
	}
	if len(created) != 3 {
		t.Fatalf("expected 3 SQL users to be created, got %#v", created)
	}
	if result.Users[string(sqlcred.ReadWrite)].Status != "created" {
		t.Fatalf("unexpected result: %#v", result)
	}
	path, err := sqlcred.CredentialsPath(home, "cluster-1")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	if !strings.Contains(string(data), "[read_only]") || !strings.Contains(string(data), "prefix.tdc_admin") {
		t.Fatalf("unexpected credentials:\n%s", string(data))
	}
	if strings.Contains(string(data), ".tdc/credentials") {
		t.Fatalf("DB credentials should not be stored in main credentials shape:\n%s", string(data))
	}
}

func TestCreateConnectionString(t *testing.T) {
	home := t.TempDir()
	writeSQLCreds(t, home, "cluster-1")
	server := clusterEndpointServer(t)
	defer server.Close()

	result, err := testSQLService(server.URL, home).CreateConnectionString(context.Background(), CreateConnectionStringOptions{
		Profile:   testProfile(),
		ClusterID: "cluster-1",
		Database:  "app",
		Format:    connectionstring.FormatJDBC,
		ReadOnly:  true,
	})
	if err != nil {
		t.Fatalf("CreateConnectionString failed: %v", err)
	}
	if result.AccessMode != sqlcred.ReadOnly || result.Username != "prefix.tdc_ro" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !strings.Contains(result.ConnectionString, "jdbc:mysql://gateway.example.com:4000/app?") ||
		!strings.Contains(result.ConnectionString, "user=prefix.tdc_ro") {
		t.Fatalf("unexpected connection string: %s", result.ConnectionString)
	}
}

func TestExecuteSQLHTTP(t *testing.T) {
	home := t.TempDir()
	writeSQLCreds(t, home, "cluster-1")
	clusterServer := clusterEndpointServer(t)
	defer clusterServer.Close()
	var sqlBody map[string]string
	sqlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "prefix.tdc_rw" || password != "rw-pass" {
			t.Fatalf("unexpected basic auth user=%q password=%q ok=%t", user, password, ok)
		}
		if err := json.NewDecoder(r.Body).Decode(&sqlBody); err != nil {
			t.Fatalf("decode SQL body: %v", err)
		}
		_, _ = w.Write([]byte(`{"types":[{"name":"n","type":"INT","nullable":false}],"rows":[["1"]]}`))
	}))
	defer sqlServer.Close()

	result, err := testSQLService(clusterServer.URL, home).withSQLHTTPBaseURL(sqlServer.URL).ExecuteSQL(context.Background(), ExecuteSQLOptions{
		Profile:   testProfile(),
		ClusterID: "cluster-1",
		SQL:       "select 1",
	})
	if err != nil {
		t.Fatalf("ExecuteSQL failed: %v", err)
	}
	if sqlBody["query"] != "select 1" {
		t.Fatalf("unexpected SQL body: %#v", sqlBody)
	}
	if result.Transport != "https" || result.AccessMode != sqlcred.ReadWrite || result.RowCount != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestConnectionRequiresPreparedCredentials(t *testing.T) {
	server := clusterEndpointServer(t)
	defer server.Close()
	_, err := testSQLService(server.URL, t.TempDir()).CreateConnectionString(context.Background(), CreateConnectionStringOptions{
		Profile:   testProfile(),
		ClusterID: "cluster-1",
	})
	if err == nil {
		t.Fatal("expected missing credentials")
	}
	if got := apperr.MessageFor(err); !strings.Contains(got, "create-db-sql-users") {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestAccessModeFlagsAreMutuallyExclusive(t *testing.T) {
	_, err := accessMode(true, true, false)
	if err == nil {
		t.Fatal("expected conflict")
	}
	if got := apperr.ExitCodeFor(err); got != 2 {
		t.Fatalf("expected exit code 2, got %d", got)
	}
}

func testSQLService(baseURL, home string) Service {
	return Service{
		Resolver: endpoints.Resolver{
			StarterBaseURL: baseURL,
			IAMBaseURL:     baseURL,
		},
		HomeDir: home,
	}
}

func (s Service) withSQLHTTPBaseURL(baseURL string) Service {
	s.SQLHTTPBaseURL = baseURL
	return s
}

func clusterEndpointServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1beta1/clusters/cluster-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"clusterId":"cluster-1","displayName":"demo","clusterPlan":"STARTER","endpoints":{"public":{"host":"gateway.example.com","port":4000}}}`))
	}))
}

func writeSQLCreds(t *testing.T, home, clusterID string) {
	t.Helper()
	if err := sqlcred.Write(home, clusterID, sqlcred.Document{
		ReadOnly:  sqlcred.Credential{Username: "prefix.tdc_ro", Password: "ro-pass"},
		ReadWrite: sqlcred.Credential{Username: "prefix.tdc_rw", Password: "rw-pass"},
		Admin:     sqlcred.Credential{Username: "prefix.tdc_admin", Password: "admin-pass"},
	}); err != nil {
		t.Fatalf("write SQL credentials: %v", err)
	}
	path, err := sqlcred.CredentialsPath(home, clusterID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, filepath.Join(".tdc", "db_users", clusterID, "credentials")) {
		t.Fatalf("unexpected SQL credentials path: %s", path)
	}
}
