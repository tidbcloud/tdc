package query

import "testing"

func TestApplyUsesJSONFieldNames(t *testing.T) {
	value := struct {
		Clusters []struct {
			ID string `json:"id"`
		} `json:"clusters"`
	}{
		Clusters: []struct {
			ID string `json:"id"`
		}{
			{ID: "cluster-1"},
		},
	}

	result, err := Apply("clusters[0].id", value)
	if err != nil {
		t.Fatalf("apply query: %v", err)
	}
	if result != "cluster-1" {
		t.Fatalf("expected cluster-1, got %#v", result)
	}
}

func TestApplyInvalidExpression(t *testing.T) {
	if _, err := Apply("clusters[", map[string]any{"clusters": []any{}}); err == nil {
		t.Fatal("expected invalid query to fail")
	}
}
