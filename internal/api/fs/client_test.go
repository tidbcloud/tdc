package fs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
