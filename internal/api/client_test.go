package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tidbcloud/tdc/internal/api/endpoints"
	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/authz"
)

func TestClientDoJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("unexpected Accept header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cluster-1"}`))
	}))
	defer server.Close()

	client := testClient(t, server.URL, authz.StarterClusterRead)
	req, err := client.NewRequest(context.Background(), http.MethodGet, "/v1beta1/clusters/cluster-1", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := client.DoJSON(req, &out); err != nil {
		t.Fatalf("DoJSON failed: %v", err)
	}
	if out.ID != "cluster-1" {
		t.Fatalf("unexpected response %#v", out)
	}
}

func TestClientMapsAuthenticationError(t *testing.T) {
	err := statusError(t, http.StatusUnauthorized, `{"message":"bad key"}`, authz.StarterClusterRead)
	if got := apperr.ExitCodeFor(err); got != 3 {
		t.Fatalf("expected exit 3, got %d", got)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected api.Error, got %T", err)
	}
	if apiErr.Code != "auth.invalid_credentials" {
		t.Fatalf("unexpected code %q", apiErr.Code)
	}
	if !strings.Contains(apperr.MessageFor(err), "authentication failed") {
		t.Fatalf("unexpected message %q", apperr.MessageFor(err))
	}
}

func TestClientMapsPermissionDenied(t *testing.T) {
	err := statusError(t, http.StatusForbidden, `{"message":"denied"}`, authz.StarterClusterCreate)
	if got := apperr.ExitCodeFor(err); got != 4 {
		t.Fatalf("expected exit 4, got %d", got)
	}
	if !strings.Contains(apperr.MessageFor(err), string(authz.StarterClusterCreate)) {
		t.Fatalf("unexpected message %q", apperr.MessageFor(err))
	}
}

func TestClientMapsNotFound(t *testing.T) {
	err := statusError(t, http.StatusNotFound, `{"message":"missing"}`, authz.StarterClusterRead)
	if got := apperr.ExitCodeFor(err); got != 5 {
		t.Fatalf("expected exit 5, got %d", got)
	}
}

func TestClientMapsAPIGap(t *testing.T) {
	err := statusError(t, http.StatusMethodNotAllowed, `{"message":"method not allowed"}`, authz.StarterSQLUserRead)
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected api.Error, got %T", err)
	}
	if apiErr.Code != "api.contract_gap" {
		t.Fatalf("unexpected code %q", apiErr.Code)
	}
}

func TestClientMapsPaymentRequired(t *testing.T) {
	err := statusError(t, http.StatusPaymentRequired, `{"message":"payment cannot be processed"}`, authz.StarterClusterCreate)
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected api.Error, got %T", err)
	}
	if apiErr.Code != "api.payment_required" {
		t.Fatalf("unexpected code %q", apiErr.Code)
	}
	if !strings.Contains(apperr.MessageFor(err), "payment required") {
		t.Fatalf("unexpected message %q", apperr.MessageFor(err))
	}
}

func statusError(t *testing.T, status int, body string, permission authz.Permission) error {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	client := testClient(t, server.URL, permission)
	client.Action = "create Starter clusters"
	req, err := client.NewRequest(context.Background(), http.MethodGet, "/v1beta1/test", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	return client.DoJSON(req, nil)
}

func testClient(t *testing.T, baseURL string, permission authz.Permission) *Client {
	t.Helper()
	client, err := New(Options{
		Endpoint: endpoints.Endpoint{
			Service:    endpoints.ServiceStarter,
			BaseURL:    baseURL,
			Provider:   "aws",
			RegionCode: "us-east-1",
		},
		ProfileName: "stage",
		Permission:  permission,
		HTTPClient:  http.DefaultClient,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	return client
}
