package db

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tidbcloud/tdc/internal/api/endpoints"
	"github.com/tidbcloud/tdc/internal/apperr"
	"github.com/tidbcloud/tdc/internal/config"
)

func TestCreateCluster(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1beta1/clusters" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["displayName"] != "demo-cluster" {
			t.Fatalf("unexpected displayName: %#v", body)
		}
		region := body["region"].(map[string]any)
		if region["name"] != "regions/aws-us-east-1" {
			t.Fatalf("unexpected region: %#v", region)
		}
		_, _ = w.Write([]byte(`{"clusterId":"cluster-1","displayName":"demo-cluster","clusterPlan":"STARTER","region":{"name":"regions/aws-us-east-1"}}`))
	}))
	defer server.Close()

	result, err := testService(server.URL).CreateCluster(context.Background(), CreateClusterOptions{
		Profile:                      testProfile(),
		DisplayName:                  "demo-cluster",
		ClusterType:                  "starter",
		ProjectID:                    "project-1",
		MonthlySpendingLimitUSDCents: -1,
	})
	if err != nil {
		t.Fatalf("CreateCluster failed: %v", err)
	}
	if result.ID != "cluster-1" || result.DisplayName != "demo-cluster" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestListClusters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageSize") != "1" {
			t.Fatalf("unexpected query %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{
			"clusters":[{"clusterId":"cluster-1","displayName":"demo-cluster","clusterPlan":"STARTER","state":"ACTIVE","region":{"name":"regions/aws-us-east-1"}}],
			"nextPageToken":"token-2",
			"totalSize":1
		}`))
	}))
	defer server.Close()

	result, err := testService(server.URL).ListClusters(context.Background(), ListClustersOptions{
		Profile:  testProfile(),
		PageSize: 1,
	})
	if err != nil {
		t.Fatalf("ListClusters failed: %v", err)
	}
	if len(result.Clusters) != 1 || result.Clusters[0].ID != "cluster-1" {
		t.Fatalf("unexpected clusters: %#v", result.Clusters)
	}
	if human := result.Human(); !strings.Contains(human, "demo-cluster") || !strings.Contains(human, "token-2") {
		t.Fatalf("unexpected text output:\n%s", human)
	}
}

func TestDescribeRejectsNonStarterCluster(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"clusterId":"cluster-1","displayName":"essential-cluster","clusterPlan":"ESSENTIAL"}`))
	}))
	defer server.Close()

	_, err := testService(server.URL).DescribeCluster(context.Background(), DescribeClusterOptions{
		Profile:   testProfile(),
		ClusterID: "cluster-1",
	})
	if err == nil {
		t.Fatal("expected non-starter cluster to fail")
	}
	if got := apperr.ExitCodeFor(err); got != 2 {
		t.Fatalf("expected exit code 2, got %d", got)
	}
}

func TestUpdateCluster(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method)
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"clusterId":"cluster-1","displayName":"demo-cluster","clusterPlan":"STARTER"}`))
		case http.MethodPatch:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["updateMask"] != "displayName,spendingLimit" {
				t.Fatalf("unexpected update mask: %#v", body)
			}
			_, _ = w.Write([]byte(`{"clusterId":"cluster-1","displayName":"renamed-cluster","clusterPlan":"STARTER"}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	result, err := testService(server.URL).UpdateCluster(context.Background(), UpdateClusterOptions{
		Profile:                      testProfile(),
		ClusterID:                    "cluster-1",
		DisplayName:                  "renamed-cluster",
		MonthlySpendingLimitUSDCents: 1000,
	})
	if err != nil {
		t.Fatalf("UpdateCluster failed: %v", err)
	}
	if result.DisplayName != "renamed-cluster" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if strings.Join(requests, ",") != "GET,PATCH" {
		t.Fatalf("unexpected requests: %#v", requests)
	}
}

func TestDeleteClusterReadsBeforeDelete(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method)
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"clusterId":"cluster-1","displayName":"demo-cluster","clusterPlan":"STARTER"}`))
		case http.MethodDelete:
			_, _ = w.Write([]byte(`{"clusterId":"cluster-1","displayName":"demo-cluster","clusterPlan":"STARTER","state":"DELETED"}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	result, err := testService(server.URL).DeleteCluster(context.Background(), DeleteClusterOptions{
		Profile:   testProfile(),
		ClusterID: "cluster-1",
	})
	if err != nil {
		t.Fatalf("DeleteCluster failed: %v", err)
	}
	if result.ID != "cluster-1" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if strings.Join(requests, ",") != "GET,DELETE" {
		t.Fatalf("unexpected requests: %#v", requests)
	}
}

func TestDryRunCreateClusterDoesNotSendRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	result, err := testService(server.URL).DryRunCreateCluster(context.Background(), "tdc db create-db-cluster", CreateClusterOptions{
		Profile:                      testProfile(),
		DisplayName:                  "demo-cluster",
		ClusterType:                  "starter",
		ProjectID:                    "project-1",
		MonthlySpendingLimitUSDCents: -1,
	})
	if err != nil {
		t.Fatalf("DryRunCreateCluster failed: %v", err)
	}
	if !result.DryRun || result.Request.Method != http.MethodPost {
		t.Fatalf("unexpected dry-run: %#v", result)
	}
	if called {
		t.Fatal("dry-run should not send a request")
	}
}

func TestCreateRequiresStarterType(t *testing.T) {
	_, err := Service{}.DryRunCreateCluster(context.Background(), "tdc db create-db-cluster", CreateClusterOptions{
		Profile:     testProfile(),
		DisplayName: "demo-cluster",
		ProjectID:   "project-1",
	})
	if err == nil {
		t.Fatal("expected missing cluster type to fail")
	}
	if got := apperr.ExitCodeFor(err); got != 2 {
		t.Fatalf("expected exit code 2, got %d", got)
	}
}

func testService(baseURL string) Service {
	return Service{
		Resolver: endpoints.Resolver{StarterBaseURL: baseURL},
	}
}

func testProfile() *config.Profile {
	return &config.Profile{
		Name:          "test",
		CloudProvider: "aws",
		RegionCode:    "us-east-1",
		TDCPublicKey:  "public",
		TDCPrivateKey: "private",
	}
}
