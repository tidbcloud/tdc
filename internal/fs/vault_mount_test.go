//go:build !windows

package fs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	gofs "github.com/hanwen/go-fuse/v2/fs"
	gofuse "github.com/hanwen/go-fuse/v2/fuse"
	"github.com/tidbcloud/tdc/internal/authz"
)

func TestVaultFuseOwnerViewExposesSecretsFieldsAndContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fs-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/vault/secrets":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"secrets": []map[string]any{
					{"name": "db-prod"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/vault/secrets/db-prod/value":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"PASSWORD": "hunter2",
				"DB_URL":   "mysql://example",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/vault/secrets/db-prod/value/PASSWORD":
			_, _ = w.Write([]byte("hunter2"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	client, ownerMode, err := testService(t.TempDir(), server.URL).vaultReadClient(dataProfile(), "", authz.FSVaultSecretRead, "test vault mount")
	if err != nil {
		t.Fatalf("vaultReadClient failed: %v", err)
	}
	if !ownerMode {
		t.Fatal("expected owner mode")
	}
	runtime := &vaultFuseRuntime{client: client, ownerMode: ownerMode}

	root := newVaultFuseNode(runtime, "", "")
	if got := dirStreamNames(t, root, context.Background()); !equalStrings(got, []string{"db-prod"}) {
		t.Fatalf("root entries = %#v", got)
	}
	secret := newVaultFuseNode(runtime, "db-prod", "")
	if got := dirStreamNames(t, secret, context.Background()); !equalStrings(got, []string{"DB_URL", "PASSWORD"}) {
		t.Fatalf("secret entries = %#v", got)
	}
	file := newVaultFuseNode(runtime, "db-prod", "PASSWORD")
	handle, _, errno := file.Open(context.Background(), 0)
	if errno != gofs.OK {
		t.Fatalf("Open errno = %v", errno)
	}
	reader, ok := handle.(gofs.FileReader)
	if !ok {
		t.Fatalf("handle does not implement FileReader")
	}
	result, errno := reader.Read(context.Background(), make([]byte, 64), 0)
	if errno != gofs.OK {
		t.Fatalf("Read errno = %v", errno)
	}
	data, status := result.Bytes(make([]byte, result.Size()))
	if status != gofuse.OK || string(data) != "hunter2" {
		t.Fatalf("Read = %q status=%v", string(data), status)
	}
}

func TestVaultFuseDelegatedViewUsesReadEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer delegated-token" {
			t.Fatalf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/vault/read":
			_ = json.NewEncoder(w).Encode(map[string][]string{"secrets": {"db-prod"}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/vault/read/db-prod":
			_ = json.NewEncoder(w).Encode(map[string]string{"TOKEN": "delegated-value"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/vault/read/db-prod/TOKEN":
			_, _ = w.Write([]byte("delegated-value"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	client, ownerMode, err := testService(t.TempDir(), server.URL).vaultReadClient(dataProfile(), "delegated-token", authz.FSVaultSecretRead, "test delegated vault mount")
	if err != nil {
		t.Fatalf("vaultReadClient failed: %v", err)
	}
	if ownerMode {
		t.Fatal("expected delegated mode")
	}
	runtime := &vaultFuseRuntime{client: client, ownerMode: ownerMode}

	root := newVaultFuseNode(runtime, "", "")
	if got := dirStreamNames(t, root, context.Background()); !equalStrings(got, []string{"db-prod"}) {
		t.Fatalf("root entries = %#v", got)
	}
	file := newVaultFuseNode(runtime, "db-prod", "TOKEN")
	handle, _, errno := file.Open(context.Background(), 0)
	if errno != gofs.OK {
		t.Fatalf("Open errno = %v", errno)
	}
	result, errno := handle.(gofs.FileReader).Read(context.Background(), make([]byte, 64), 0)
	if errno != gofs.OK {
		t.Fatalf("Read errno = %v", errno)
	}
	data, status := result.Bytes(make([]byte, result.Size()))
	if status != gofuse.OK || string(data) != "delegated-value" {
		t.Fatalf("Read = %q status=%v", string(data), status)
	}
}

func dirStreamNames(t *testing.T, node *vaultFuseNode, ctx context.Context) []string {
	t.Helper()
	stream, errno := node.Readdir(ctx)
	if errno != gofs.OK {
		t.Fatalf("Readdir errno = %v", errno)
	}
	defer stream.Close()
	var names []string
	for stream.HasNext() {
		entry, errno := stream.Next()
		if errno != gofs.OK {
			t.Fatalf("DirStream Next errno = %v", errno)
		}
		names = append(names, entry.Name)
	}
	sort.Strings(names)
	return names
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
