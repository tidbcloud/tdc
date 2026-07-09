package db

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tidbcloud/tdc/internal/apperr"
)

func TestCreateBranch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1beta1/clusters/cluster-1/branches" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["displayName"] != "dev" {
			t.Fatalf("unexpected body: %#v", body)
		}
		_, _ = w.Write([]byte(`{"branchId":"branch-1","displayName":"dev","clusterId":"cluster-1","state":"CREATING"}`))
	}))
	defer server.Close()

	result, err := testService(server.URL).CreateBranch(context.Background(), CreateBranchOptions{
		Profile:     testProfile(),
		ClusterID:   "cluster-1",
		DisplayName: "dev",
	})
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if result.ID != "branch-1" || result.DisplayName != "dev" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestListBranches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageSize") != "1" {
			t.Fatalf("unexpected query %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{
			"branches":[{"branchId":"branch-1","displayName":"dev","clusterId":"cluster-1","state":"ACTIVE"}],
			"nextPageToken":"token-2",
			"totalSize":1
		}`))
	}))
	defer server.Close()

	result, err := testService(server.URL).ListBranches(context.Background(), ListBranchesOptions{
		Profile:   testProfile(),
		ClusterID: "cluster-1",
		PageSize:  1,
	})
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}
	if len(result.Branches) != 1 || result.Branches[0].ID != "branch-1" {
		t.Fatalf("unexpected branches: %#v", result.Branches)
	}
	if human := result.Human(); !strings.Contains(human, "dev") || !strings.Contains(human, "token-2") {
		t.Fatalf("unexpected human output:\n%s", human)
	}
}

func TestDescribeBranch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1beta1/clusters/cluster-1/branches/branch-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("view") != "FULL" {
			t.Fatalf("unexpected view %q", r.URL.Query().Get("view"))
		}
		_, _ = w.Write([]byte(`{"branchId":"branch-1","displayName":"dev","clusterId":"cluster-1","state":"ACTIVE"}`))
	}))
	defer server.Close()

	result, err := testService(server.URL).DescribeBranch(context.Background(), DescribeBranchOptions{
		Profile:   testProfile(),
		ClusterID: "cluster-1",
		BranchID:  "branch-1",
		View:      "FULL",
	})
	if err != nil {
		t.Fatalf("DescribeBranch failed: %v", err)
	}
	if result.ID != "branch-1" || result.State != "ACTIVE" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestDeleteBranchRequiresMatchingConfirmation(t *testing.T) {
	deleteCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalled = true
		}
		_, _ = w.Write([]byte(`{"branchId":"branch-1","displayName":"dev","clusterId":"cluster-1","state":"ACTIVE"}`))
	}))
	defer server.Close()

	_, err := testService(server.URL).DeleteBranch(context.Background(), DeleteBranchOptions{
		Profile:                    testProfile(),
		ClusterID:                  "cluster-1",
		BranchID:                   "branch-1",
		ConfirmDBClusterBranchName: "wrong-name",
	})
	if err == nil {
		t.Fatal("expected confirmation mismatch to fail")
	}
	if deleteCalled {
		t.Fatal("delete should not be called when confirmation mismatches")
	}
}

func TestDeleteBranch(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method)
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"branchId":"branch-1","displayName":"dev","clusterId":"cluster-1","state":"ACTIVE"}`))
		case http.MethodDelete:
			_, _ = w.Write([]byte(`{"branchId":"branch-1","displayName":"dev","clusterId":"cluster-1","state":"DELETED"}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	result, err := testService(server.URL).DeleteBranch(context.Background(), DeleteBranchOptions{
		Profile:                    testProfile(),
		ClusterID:                  "cluster-1",
		BranchID:                   "branch-1",
		ConfirmDBClusterBranchName: "dev",
	})
	if err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}
	if result.State != "DELETED" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if strings.Join(requests, ",") != "GET,DELETE" {
		t.Fatalf("unexpected requests: %#v", requests)
	}
}

func TestDryRunCreateBranchDoesNotSendRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	result, err := testService(server.URL).DryRunCreateBranch(context.Background(), "tdc db create-db-cluster-branch", CreateBranchOptions{
		Profile:     testProfile(),
		ClusterID:   "cluster-1",
		DisplayName: "dev",
	})
	if err != nil {
		t.Fatalf("DryRunCreateBranch failed: %v", err)
	}
	if !result.DryRun || result.Request.Method != http.MethodPost {
		t.Fatalf("unexpected dry-run: %#v", result)
	}
	if called {
		t.Fatal("dry-run should not send a request")
	}
}

func TestDryRunDeleteBranchDoesNotSendRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	result, err := testService(server.URL).DryRunDeleteBranch(context.Background(), "tdc db delete-db-cluster-branch", DeleteBranchOptions{
		Profile:                    testProfile(),
		ClusterID:                  "cluster-1",
		BranchID:                   "branch-1",
		ConfirmDBClusterBranchName: "dev",
	})
	if err != nil {
		t.Fatalf("DryRunDeleteBranch failed: %v", err)
	}
	if !result.DryRun || result.Request.Method != http.MethodDelete {
		t.Fatalf("unexpected dry-run: %#v", result)
	}
	if called {
		t.Fatal("dry-run should not send a request")
	}
}

func TestCreateBranchRequiresName(t *testing.T) {
	_, err := Service{}.DryRunCreateBranch(context.Background(), "tdc db create-db-cluster-branch", CreateBranchOptions{
		Profile:   testProfile(),
		ClusterID: "cluster-1",
	})
	if err == nil {
		t.Fatal("expected missing branch name to fail")
	}
	if got := apperr.ExitCodeFor(err); got != 2 {
		t.Fatalf("expected exit code 2, got %d", got)
	}
}
