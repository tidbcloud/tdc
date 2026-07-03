package fs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Icemap/tdc/internal/api"
	"github.com/Icemap/tdc/internal/api/endpoints"
	"github.com/Icemap/tdc/internal/authz"
)

func TestProvisionAndDeleteTenant(t *testing.T) {
	var sawProvision bool
	var sawDelete bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/provision":
			sawProvision = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode provision body: %v", err)
			}
			if body["public_key"] != "public" || body["private_key"] != "private" || body["tidbcloud_spending_limit"] != float64(0) {
				t.Fatalf("unexpected provision body: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(ProvisionResponse{
				TenantID: "tenant-1",
				APIKey:   "fs-secret",
				Status:   "active",
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/tenant":
			sawDelete = true
			var body DeprovisionRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode delete body: %v", err)
			}
			if body.PublicKey != "public" || body.PrivateKey != "private" {
				t.Fatalf("unexpected delete body: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(DeleteResponse{Status: "deleting"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := testClient(t, server.URL)
	spendingLimit := int64(0)
	provision, err := client.Provision(context.Background(), ProvisionRequest{
		PublicKey:              "public",
		PrivateKey:             "private",
		TiDBCloudSpendingLimit: &spendingLimit,
	})
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	if provision.TenantID != "tenant-1" || provision.APIKey != "fs-secret" {
		t.Fatalf("unexpected provision response: %#v", provision)
	}

	deleted, err := client.DeleteTenant(context.Background(), DeprovisionRequest{
		PublicKey:  "public",
		PrivateKey: "private",
	})
	if err != nil {
		t.Fatalf("DeleteTenant failed: %v", err)
	}
	if deleted.Status != "deleting" {
		t.Fatalf("unexpected delete response: %#v", deleted)
	}
	if !sawProvision || !sawDelete {
		t.Fatalf("expected provision and delete to be called")
	}
}

func TestStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/status" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(StatusResponse{
			Status:   "ok",
			TenantID: "tenant-1",
			Kind:     "live",
		})
	}))
	defer server.Close()

	status, err := testClient(t, server.URL).Status(context.Background())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.Status != "ok" || status.TenantID != "tenant-1" || status.Kind != "live" {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestDataPlaneMethods(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/v1/fs/workspace/read me.txt":
			if got := r.Header.Get("Content-Type"); got != "application/octet-stream" {
				t.Fatalf("Content-Type = %q", got)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if string(body) != "hello" {
				t.Fatalf("unexpected write body %q", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(WriteResponse{Revision: 7})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace/read me.txt" && r.URL.RawQuery == "":
			_, _ = w.Write([]byte("raw bytes"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace" && r.URL.Query().Get("list") == "1":
			_ = json.NewEncoder(w).Encode(ListResponse{Entries: []FileInfo{{Name: "read me.txt", Size: 9, IsDir: false, Mtime: 1700000000}}})
		case r.Method == http.MethodHead && r.URL.Path == "/v1/fs/workspace/read me.txt":
			w.Header().Set("Content-Length", "9")
			w.Header().Set("X-Dat9-Revision", "11")
			w.Header().Set("X-Dat9-Mtime", "1700000000")
			w.Header().Set("X-Dat9-Mode", "420")
			w.Header().Set("X-Dat9-IsDir", "false")
			w.Header().Set("X-Dat9-Nlink", "1")
			w.Header().Set("X-Dat9-Resource-Id", "resource-1")
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace/read me.txt" && r.URL.Query().Get("stat") == "1":
			_ = json.NewEncoder(w).Encode(StatMetadataResponse{
				Size:        9,
				IsDir:       false,
				ResourceID:  "resource-1",
				Nlink:       1,
				Revision:    12,
				Mtime:       1700000001,
				ContentType: "text/plain",
				Tags:        map[string]string{"kind": "doc"},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/fs/workspace/read me.txt" && r.URL.Query().Get("recursive") == "1":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/fs/workspace/copy.txt" && hasRawQueryKey(r.URL, "copy"):
			if got := r.Header.Get("X-Dat9-Copy-Source"); got != "/workspace/read me.txt" {
				t.Fatalf("copy source = %q", got)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/fs/workspace/moved.txt" && hasRawQueryKey(r.URL, "rename"):
			if got := r.Header.Get("X-Dat9-Rename-Source"); got != "/workspace/read me.txt" {
				t.Fatalf("rename source = %q", got)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/fs/workspace/newdir" && hasRawQueryKey(r.URL, "mkdir") && r.URL.Query().Get("mode") == "700":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace" && r.URL.Query().Get("grep") == "needle" && r.URL.Query().Get("limit") == "5":
			_ = json.NewEncoder(w).Encode([]SearchResult{{Path: "/workspace/read me.txt", Name: "read me.txt", SizeBytes: 9}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/fs/workspace" && hasRawQueryKey(r.URL, "find"):
			if r.URL.Query().Get("name") != "*.txt" || r.URL.Query().Get("type") != "file" || r.URL.Query().Get("minsize") != "1" {
				t.Fatalf("unexpected find query %q", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode([]SearchResult{{Path: "/workspace/read me.txt", Name: "read me.txt", SizeBytes: 9}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	client := testClient(t, server.URL)
	ctx := context.Background()
	written, err := client.WriteFile(ctx, "/workspace/read me.txt", []byte("hello"))
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if written.Revision != 7 {
		t.Fatalf("unexpected write response: %#v", written)
	}
	data, err := client.ReadFile(ctx, "/workspace/read me.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "raw bytes" {
		t.Fatalf("unexpected read bytes %q", data)
	}
	list, err := client.List(ctx, "/workspace")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list.Entries) != 1 || list.Entries[0].Name != "read me.txt" {
		t.Fatalf("unexpected list response: %#v", list)
	}
	stat, err := client.Stat(ctx, "/workspace/read me.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if stat.SizeBytes != 9 || stat.Revision != 11 || stat.ResourceID != "resource-1" {
		t.Fatalf("unexpected stat: %#v", stat)
	}
	metadata, err := client.StatMetadata(ctx, "/workspace/read me.txt")
	if err != nil {
		t.Fatalf("StatMetadata failed: %v", err)
	}
	if metadata.ContentType != "text/plain" || metadata.Tags["kind"] != "doc" {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	if err := client.DeleteFile(ctx, "/workspace/read me.txt", true); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}
	if err := client.CopyRemote(ctx, "/workspace/read me.txt", "/workspace/copy.txt"); err != nil {
		t.Fatalf("CopyRemote failed: %v", err)
	}
	if err := client.Rename(ctx, "/workspace/read me.txt", "/workspace/moved.txt"); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
	if err := client.Mkdir(ctx, "/workspace/newdir", 0o700); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	grep, err := client.Grep(ctx, "/workspace", "needle", 5)
	if err != nil {
		t.Fatalf("Grep failed: %v", err)
	}
	if len(grep) != 1 || grep[0].Path != "/workspace/read me.txt" {
		t.Fatalf("unexpected grep response: %#v", grep)
	}
	params := url.Values{}
	params.Set("name", "*.txt")
	params.Set("type", "file")
	params.Set("minsize", "1")
	find, err := client.Find(ctx, "/workspace", params)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(find) != 1 || find[0].Name != "read me.txt" {
		t.Fatalf("unexpected find response: %#v", find)
	}
	if len(calls) != 11 {
		t.Fatalf("expected 11 calls, got %d: %#v", len(calls), calls)
	}
}

func testClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	client, err := api.New(api.Options{
		Endpoint: endpoints.Endpoint{
			Service:    endpoints.ServiceFS,
			BaseURL:    baseURL,
			Provider:   "aws",
			RegionCode: "us-east-1",
		},
		ProfileName: "test",
		Permission:  authz.FSVolumeCreate,
	})
	if err != nil {
		t.Fatal(err)
	}
	return New(client)
}

func hasRawQueryKey(u *url.URL, key string) bool {
	for _, part := range strings.Split(u.RawQuery, "&") {
		if part == key || strings.HasPrefix(part, key+"=") {
			return true
		}
	}
	return false
}
