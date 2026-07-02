package iam

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Icemap/tdc/internal/api"
	"github.com/Icemap/tdc/internal/api/endpoints"
	"github.com/Icemap/tdc/internal/apperr"
	"github.com/Icemap/tdc/internal/authz"
)

func TestListProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta1/projects" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("pageSize"); got != "2" {
			t.Fatalf("unexpected pageSize %q", got)
		}
		if got := r.URL.Query().Get("pageToken"); got != "token-1" {
			t.Fatalf("unexpected pageToken %q", got)
		}
		_, _ = w.Write([]byte(`{
			"projects": [
				{
					"id": "project-1",
					"name": "Project 1",
					"org_id": "org-1",
					"cluster_count": 3,
					"user_count": 4,
					"create_timestamp": "1688460316",
					"aws_cmek_enabled": true
				}
			],
			"nextPageToken": "token-2"
		}`))
	}))
	defer server.Close()

	client := New(newTestAPIClient(t, server.URL))
	response, err := client.ListProjects(context.Background(), ListProjectsOptions{
		PageSize:  2,
		PageToken: "token-1",
	})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if response.NextPageToken != "token-2" {
		t.Fatalf("unexpected next page token %q", response.NextPageToken)
	}
	if len(response.Projects) != 1 || response.Projects[0].ID != "project-1" {
		t.Fatalf("unexpected projects: %#v", response.Projects)
	}
	if !response.Projects[0].AWSCMEKEnabled {
		t.Fatalf("expected aws_cmek_enabled to be true")
	}
}

func TestListProjectsAcceptsSnakeCaseNextPageToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"projects":[],"next_page_token":"snake-token"}`))
	}))
	defer server.Close()

	client := New(newTestAPIClient(t, server.URL))
	response, err := client.ListProjects(context.Background(), ListProjectsOptions{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if response.NextPageToken != "snake-token" {
		t.Fatalf("unexpected next page token %q", response.NextPageToken)
	}
	if response.Projects == nil {
		t.Fatal("expected projects to be an empty slice, not nil")
	}
}

func TestListProjectsMapsAuthAndPermissionErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		exitCode   int
	}{
		{name: "unauthenticated", statusCode: http.StatusUnauthorized, exitCode: 3},
		{name: "permission denied", statusCode: http.StatusForbidden, exitCode: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"message":"denied"}`))
			}))
			defer server.Close()

			client := New(newTestAPIClient(t, server.URL))
			_, err := client.ListProjects(context.Background(), ListProjectsOptions{})
			if err == nil {
				t.Fatal("expected ListProjects to fail")
			}
			if got := apperr.ExitCodeFor(err); got != tt.exitCode {
				t.Fatalf("expected exit code %d, got %d", tt.exitCode, got)
			}
		})
	}
}

func newTestAPIClient(t *testing.T, baseURL string) *api.Client {
	t.Helper()
	client, err := api.New(api.Options{
		Endpoint: endpoints.Endpoint{
			Service: endpoints.ServiceIAM,
			BaseURL: baseURL,
		},
		ProfileName: "test",
		Permission:  authz.OrganizationProjectRead,
		HTTPClient:  http.DefaultClient,
	})
	if err != nil {
		t.Fatalf("new API client: %v", err)
	}
	return client
}
