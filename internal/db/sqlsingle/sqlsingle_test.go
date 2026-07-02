package sqlsingle

import "testing"

func TestValidateSingleStatement(t *testing.T) {
	valid := []string{
		"select 1",
		"select 1;",
		"select ';'",
		"select `semi;colon` from t",
	}
	for _, statement := range valid {
		if err := Validate(statement); err != nil {
			t.Fatalf("expected %q to be valid: %v", statement, err)
		}
	}
	invalid := []string{
		"",
		"select 1; select 2",
		"select 1;;",
		"select 'unterminated",
	}
	for _, statement := range invalid {
		if err := Validate(statement); err == nil {
			t.Fatalf("expected %q to be invalid", statement)
		}
	}
}
