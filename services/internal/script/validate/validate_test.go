package validate

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// ValidateSource — valid scripts
// ---------------------------------------------------------------------------

func TestValidScript(t *testing.T) {
	src := `
SET x 5
SET y 10
IF x < y
  LOG INFO "x is smaller"
ENDIF
`
	res := ValidateSource(src)
	if !res.Valid {
		t.Errorf("expected valid, got errors: %+v", res.Errors)
	}
	if len(res.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(res.Errors))
	}
}

func TestValidTestBlock(t *testing.T) {
	src := `
TEST "voltage check"
  SET v 5.0
  ASSERT v > 4.5 "voltage too low"
  PASS "ok"
ENDTEST
`
	res := ValidateSource(src)
	if !res.Valid {
		t.Errorf("expected valid, got errors: %+v", res.Errors)
	}
}

func TestValidSuiteBlock(t *testing.T) {
	src := `
SUITE "power tests"
  TEST "5v rail"
    PASS "ok"
  ENDTEST
ENDSUITE
`
	res := ValidateSource(src)
	if !res.Valid {
		t.Errorf("expected valid, got errors: %+v", res.Errors)
	}
}

func TestValidFunctionDef(t *testing.T) {
	src := `
FUNCTION add(a, b)
  RETURN a + b
ENDFUNCTION
`
	res := ValidateSource(src)
	if !res.Valid {
		t.Errorf("expected valid, got errors: %+v", res.Errors)
	}
}

func TestValidLoopAndWhile(t *testing.T) {
	src := `
LOOP 3 TIMES AS i
  LOG INFO i
ENDLOOP

SET n 0
WHILE n < 5
  SET n n + 1
ENDWHILE
`
	res := ValidateSource(src)
	if !res.Valid {
		t.Errorf("expected valid, got errors: %+v", res.Errors)
	}
}

// ---------------------------------------------------------------------------
// ValidateSource — lex errors
// ---------------------------------------------------------------------------

func TestLexErrorUnterminatedString(t *testing.T) {
	src := `SET x "hello`
	res := ValidateSource(src)
	if res.Valid {
		t.Error("expected invalid for unterminated string")
	}
	if len(res.Errors) == 0 {
		t.Fatal("expected at least 1 error")
	}
	if res.Errors[0].Severity != "error" {
		t.Errorf("severity = %q, want %q", res.Errors[0].Severity, "error")
	}
	if res.Errors[0].Line != 1 {
		t.Errorf("line = %d, want 1", res.Errors[0].Line)
	}
}

// ---------------------------------------------------------------------------
// ValidateSource — parse errors
// ---------------------------------------------------------------------------

func TestParseErrorMissingEndif(t *testing.T) {
	src := `
IF TRUE
  SET x 1
`
	res := ValidateSource(src)
	if res.Valid {
		t.Error("expected invalid for missing ENDIF")
	}
	if len(res.Errors) == 0 {
		t.Fatal("expected at least 1 parse error")
	}
}

func TestParseErrorMissingEndtest(t *testing.T) {
	src := `
TEST "incomplete"
  SET x 1
`
	res := ValidateSource(src)
	if res.Valid {
		t.Error("expected invalid for missing ENDTEST")
	}
	if len(res.Errors) == 0 {
		t.Fatal("expected at least 1 parse error")
	}
}

// ---------------------------------------------------------------------------
// Error position and context
// ---------------------------------------------------------------------------

func TestErrorHasContext(t *testing.T) {
	src := `SET x "unterminated`
	res := ValidateSource(src)
	if res.Valid {
		t.Fatal("expected invalid")
	}
	if len(res.Errors) == 0 {
		t.Fatal("expected errors")
	}
	// Context should contain the source line.
	if res.Errors[0].Context == "" {
		t.Error("expected non-empty context")
	}
}

func TestMultipleErrors(t *testing.T) {
	// Multiple lines with issues — at minimum the parser should report errors.
	src := `
IF TRUE
SET x 1
`
	res := ValidateSource(src)
	if res.Valid {
		t.Error("expected invalid")
	}
	// Should have at least one error (missing ENDIF).
	if len(res.Errors) == 0 {
		t.Fatal("expected at least 1 error")
	}
}

// ---------------------------------------------------------------------------
// ValidateFile
// ---------------------------------------------------------------------------

func TestValidateFileValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.art")
	if err := os.WriteFile(path, []byte("SET x 5\n"), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := ValidateFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Valid {
		t.Errorf("expected valid, got errors: %+v", res.Errors)
	}
}

func TestValidateFileInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.art")
	if err := os.WriteFile(path, []byte("SET x \"unterminated\n"), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := ValidateFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Valid {
		t.Error("expected invalid for unterminated string")
	}
}

func TestValidateFileNotFound(t *testing.T) {
	_, err := ValidateFile("/nonexistent/path/to/script.art")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// Empty source
// ---------------------------------------------------------------------------

func TestValidateEmptySource(t *testing.T) {
	res := ValidateSource("")
	if !res.Valid {
		t.Errorf("empty source should be valid, got errors: %+v", res.Errors)
	}
}

// ---------------------------------------------------------------------------
// contextLine helper
// ---------------------------------------------------------------------------

func TestContextLineInRange(t *testing.T) {
	lines := []string{"first", "second", "third"}
	if got := contextLine(lines, 2); got != "second" {
		t.Errorf("contextLine(2) = %q, want %q", got, "second")
	}
}

func TestContextLineOutOfRange(t *testing.T) {
	lines := []string{"first"}
	if got := contextLine(lines, 0); got != "" {
		t.Errorf("contextLine(0) = %q, want empty", got)
	}
	if got := contextLine(lines, 5); got != "" {
		t.Errorf("contextLine(5) = %q, want empty", got)
	}
}
