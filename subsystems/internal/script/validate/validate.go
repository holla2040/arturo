// Package validate provides parse-only validation for .art script files,
// producing structured JSON-friendly error output suitable for LLM consumption.
package validate

import (
	"os"
	"strings"

	"github.com/holla2040/arturo/internal/script/lexer"
	"github.com/holla2040/arturo/internal/script/parser"
)

// ValidationError describes a single error found during validation.
type ValidationError struct {
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity string `json:"severity"` // "error" or "warning"
	Message  string `json:"message"`
	Context  string `json:"context,omitempty"` // source line for reference
}

// ValidationResult is the outcome of validating a script source.
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// ValidateSource runs the lexer and parser on source, collecting all errors.
func ValidateSource(source string) *ValidationResult {
	result := &ValidationResult{Valid: true}
	lines := strings.Split(source, "\n")

	// Run lexer.
	tokens, lexErrs := lexer.New(source).Tokenize()
	for _, le := range lexErrs {
		ctx := contextLine(lines, le.Line)
		result.Errors = append(result.Errors, ValidationError{
			Line:     le.Line,
			Column:   le.Column,
			Severity: "error",
			Message:  le.Message,
			Context:  ctx,
		})
	}

	if len(lexErrs) > 0 {
		result.Valid = false
		return result
	}

	// Run parser.
	_, parseErrs := parser.New(tokens).Parse()
	for _, pe := range parseErrs {
		ctx := contextLine(lines, pe.Line)
		result.Errors = append(result.Errors, ValidationError{
			Line:     pe.Line,
			Column:   pe.Column,
			Severity: pe.Severity,
			Message:  pe.Message,
			Context:  ctx,
		})
	}

	if len(parseErrs) > 0 {
		result.Valid = false
	}

	return result
}

// ValidateFile reads the given file path and validates its contents.
func ValidateFile(path string) (*ValidationResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ValidateSource(string(data)), nil
}

// contextLine returns the source line at the given 1-based line number, or ""
// if out of range.
func contextLine(lines []string, line int) string {
	if line > 0 && line <= len(lines) {
		return lines[line-1]
	}
	return ""
}
