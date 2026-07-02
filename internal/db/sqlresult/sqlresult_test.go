package sqlresult

import "testing"

func TestHumanRendersTable(t *testing.T) {
	result := Result{
		Fields: []Field{
			{Name: "one", Type: "BIGINT"},
			{Name: "today", Type: "DATE"},
		},
		Rows: []map[string]any{
			{"one": int64(1), "today": "2026-07-02"},
		},
		RowCount: 1,
	}

	want := `+-----+------------+
| one | today      |
+-----+------------+
| 1   | 2026-07-02 |
+-----+------------+
1 row in set`
	if got := result.Human(); got != want {
		t.Fatalf("unexpected human output:\n%s", got)
	}
}

func TestHumanRendersNullsAndEscapesControlCharacters(t *testing.T) {
	result := Result{
		Fields: []Field{
			{Name: "name"},
			{Name: "note"},
		},
		Rows: []map[string]any{
			{"name": nil, "note": "line 1\nline 2\tend"},
		},
		RowCount: 1,
	}

	want := `+------+---------------------+
| name | note                |
+------+---------------------+
| NULL | line 1\nline 2\tend |
+------+---------------------+
1 row in set`
	if got := result.Human(); got != want {
		t.Fatalf("unexpected human output:\n%s", got)
	}
}

func TestHumanRendersRowsAffected(t *testing.T) {
	rowsAffected := int64(2)
	lastInsertID := "42"
	result := Result{
		RowsAffected: &rowsAffected,
		LastInsertID: &lastInsertID,
	}

	want := "Query OK, 2 rows affected, last insert id: 42"
	if got := result.Human(); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
