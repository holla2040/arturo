package lexer

import (
	"fmt"
	"testing"

	"github.com/holla2040/arturo/internal/script/token"
)

// helper to assert no lex errors were produced.
func requireNoErrors(t *testing.T, errs []LexError) {
	t.Helper()
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("unexpected lex error: %s", e.Error())
		}
		t.FailNow()
	}
}

// helper to assert token types match expectations (ignoring positions/literals).
func requireTypes(t *testing.T, tokens []token.Token, expected []token.TokenType) {
	t.Helper()
	if len(tokens) != len(expected) {
		t.Fatalf("token count mismatch: got %d, want %d\ngot:  %s\nwant: %s",
			len(tokens), len(expected), fmtTypes(tokens), fmtExpected(expected))
	}
	for i, tt := range expected {
		if tokens[i].Type != tt {
			t.Errorf("token[%d]: got %s (%q), want %s",
				i, tokens[i].Type, tokens[i].Literal, tt)
		}
	}
}

func fmtTypes(tokens []token.Token) string {
	var s string
	for i, t := range tokens {
		if i > 0 {
			s += ", "
		}
		s += t.Type.String()
	}
	return s
}

func fmtExpected(types []token.TokenType) string {
	var s string
	for i, t := range types {
		if i > 0 {
			s += ", "
		}
		s += t.String()
	}
	return s
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestEmptyInput(t *testing.T) {
	tokens, errs := New("").Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{token.TOKEN_EOF})
}

func TestWhitespaceOnly(t *testing.T) {
	tokens, errs := New("   \t  ").Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{token.TOKEN_EOF})
}

func TestKeywordsCaseInsensitive(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  token.TokenType
	}{
		{"uppercase SET", "SET", token.TOKEN_SET},
		{"titlecase Set", "Set", token.TOKEN_SET},
		{"lowercase set", "set", token.TOKEN_SET},
		{"mixed sEt", "sEt", token.TOKEN_SET},
		{"uppercase IF", "IF", token.TOKEN_IF},
		{"lowercase if", "if", token.TOKEN_IF},
		{"uppercase ENDTEST", "ENDTEST", token.TOKEN_ENDTEST},
		{"lowercase endtest", "endtest", token.TOKEN_ENDTEST},
		{"uppercase FOREACH", "FOREACH", token.TOKEN_FOREACH},
		{"lowercase foreach", "foreach", token.TOKEN_FOREACH},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tokens, errs := New(tc.input).Tokenize()
			requireNoErrors(t, errs)
			if len(tokens) < 1 {
				t.Fatalf("expected at least 1 token")
			}
			if tokens[0].Type != tc.want {
				t.Errorf("got %s, want %s", tokens[0].Type, tc.want)
			}
			if tokens[0].Literal != tc.input {
				t.Errorf("literal: got %q, want %q", tokens[0].Literal, tc.input)
			}
		})
	}
}

func TestAllKeywords(t *testing.T) {
	// Spot-check a selection of keywords to make sure the map is wired up.
	cases := []struct {
		input string
		want  token.TokenType
	}{
		{"TEST", token.TOKEN_TEST},
		{"ENDTEST", token.TOKEN_ENDTEST},
		{"SUITE", token.TOKEN_SUITE},
		{"ENDSUITE", token.TOKEN_ENDSUITE},
		{"SETUP", token.TOKEN_SETUP},
		{"ENDSETUP", token.TOKEN_ENDSETUP},
		{"TEARDOWN", token.TOKEN_TEARDOWN},
		{"ENDTEARDOWN", token.TOKEN_ENDTEARDOWN},
		{"CONST", token.TOKEN_CONST},
		{"GLOBAL", token.TOKEN_GLOBAL},
		{"DELETE", token.TOKEN_DELETE},
		{"APPEND", token.TOKEN_APPEND},
		{"EXTEND", token.TOKEN_EXTEND},
		{"ELSEIF", token.TOKEN_ELSEIF},
		{"ELSE", token.TOKEN_ELSE},
		{"ENDIF", token.TOKEN_ENDIF},
		{"LOOP", token.TOKEN_LOOP},
		{"TIMES", token.TOKEN_TIMES},
		{"AS", token.TOKEN_AS},
		{"ENDLOOP", token.TOKEN_ENDLOOP},
		{"WHILE", token.TOKEN_WHILE},
		{"ENDWHILE", token.TOKEN_ENDWHILE},
		{"IN", token.TOKEN_IN},
		{"ENDFOREACH", token.TOKEN_ENDFOREACH},
		{"BREAK", token.TOKEN_BREAK},
		{"CONTINUE", token.TOKEN_CONTINUE},
		{"TRY", token.TOKEN_TRY},
		{"CATCH", token.TOKEN_CATCH},
		{"FINALLY", token.TOKEN_FINALLY},
		{"ENDTRY", token.TOKEN_ENDTRY},
		{"PARALLEL", token.TOKEN_PARALLEL},
		{"ENDPARALLEL", token.TOKEN_ENDPARALLEL},
		{"TIMEOUT", token.TOKEN_TIMEOUT},
		{"CONNECT", token.TOKEN_CONNECT},
		{"DISCONNECT", token.TOKEN_DISCONNECT},
		{"TCP", token.TOKEN_TCP},
		{"SERIAL", token.TOKEN_SERIAL},
		{"ALL", token.TOKEN_ALL},
		{"SEND", token.TOKEN_SEND},
		{"QUERY", token.TOKEN_QUERY},
		{"RELAY", token.TOKEN_RELAY},
		{"FUNCTION", token.TOKEN_FUNCTION},
		{"ENDFUNCTION", token.TOKEN_ENDFUNCTION},
		{"CALL", token.TOKEN_CALL},
		{"RETURN", token.TOKEN_RETURN},
		{"IMPORT", token.TOKEN_IMPORT},
		{"LIBRARY", token.TOKEN_LIBRARY},
		{"ENDLIBRARY", token.TOKEN_ENDLIBRARY},
		{"PASS", token.TOKEN_PASS},
		{"FAIL", token.TOKEN_FAIL},
		{"SKIP", token.TOKEN_SKIP},
		{"ASSERT", token.TOKEN_ASSERT},
		{"LOG", token.TOKEN_LOG},
		{"DELAY", token.TOKEN_DELAY},
		{"RESERVE", token.TOKEN_RESERVE},
		{"ON", token.TOKEN_ON},
		{"OFF", token.TOKEN_OFF},
		{"TOGGLE", token.TOKEN_TOGGLE},
		{"GET", token.TOKEN_GET},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			tokens, errs := New(tc.input).Tokenize()
			requireNoErrors(t, errs)
			if tokens[0].Type != tc.want {
				t.Errorf("got %s, want %s", tokens[0].Type, tc.want)
			}
		})
	}
}

func TestIntegerLiteral(t *testing.T) {
	tokens, errs := New("42").Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{token.TOKEN_INT, token.TOKEN_EOF})
	if tokens[0].Literal != "42" {
		t.Errorf("literal: got %q, want %q", tokens[0].Literal, "42")
	}
}

func TestFloatLiteral(t *testing.T) {
	tokens, errs := New("3.14").Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{token.TOKEN_FLOAT, token.TOKEN_EOF})
	if tokens[0].Literal != "3.14" {
		t.Errorf("literal: got %q, want %q", tokens[0].Literal, "3.14")
	}
}

func TestScientificNotation(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		literal string
	}{
		{"lowercase e negative", "1.5e-3", "1.5e-3"},
		{"uppercase E positive", "2E+10", "2E+10"},
		{"e without sign", "1e5", "1e5"},
		{"float with e", "6.022e23", "6.022e23"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tokens, errs := New(tc.input).Tokenize()
			requireNoErrors(t, errs)
			if tokens[0].Type != token.TOKEN_FLOAT {
				t.Errorf("type: got %s, want FLOAT", tokens[0].Type)
			}
			if tokens[0].Literal != tc.literal {
				t.Errorf("literal: got %q, want %q", tokens[0].Literal, tc.literal)
			}
		})
	}
}

func TestStringLiteral(t *testing.T) {
	tokens, errs := New(`"hello"`).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{token.TOKEN_STRING, token.TOKEN_EOF})
	if tokens[0].Literal != "hello" {
		t.Errorf("literal: got %q, want %q", tokens[0].Literal, "hello")
	}
}

func TestStringEscapeSequences(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"escaped quote", `"say \"hi\""`, "say \"hi\""},
		{"escaped backslash", `"path\\to"`, "path\\to"},
		{"escaped newline", `"line1\nline2"`, "line1\nline2"},
		{"escaped tab", `"col1\tcol2"`, "col1\tcol2"},
		{"mixed escapes", `"a\tb\nc"`, "a\tb\nc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tokens, errs := New(tc.input).Tokenize()
			requireNoErrors(t, errs)
			if tokens[0].Type != token.TOKEN_STRING {
				t.Fatalf("type: got %s, want STRING", tokens[0].Type)
			}
			if tokens[0].Literal != tc.want {
				t.Errorf("literal: got %q, want %q", tokens[0].Literal, tc.want)
			}
		})
	}
}

func TestEmptyString(t *testing.T) {
	tokens, errs := New(`""`).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{token.TOKEN_STRING, token.TOKEN_EOF})
	if tokens[0].Literal != "" {
		t.Errorf("literal: got %q, want empty", tokens[0].Literal)
	}
}

func TestUnterminatedString(t *testing.T) {
	tokens, errs := New(`"hello`).Tokenize()
	if len(errs) == 0 {
		t.Fatal("expected a lex error for unterminated string")
	}
	if tokens[0].Type != token.TOKEN_ILLEGAL {
		t.Errorf("type: got %s, want ILLEGAL", tokens[0].Type)
	}
}

func TestUnterminatedStringNewline(t *testing.T) {
	tokens, errs := New("\"hello\nworld\"").Tokenize()
	if len(errs) == 0 {
		t.Fatal("expected a lex error for unterminated string at newline")
	}
	if tokens[0].Type != token.TOKEN_ILLEGAL {
		t.Errorf("type: got %s, want ILLEGAL", tokens[0].Type)
	}
}

func TestSingleCharOperators(t *testing.T) {
	cases := []struct {
		input string
		want  token.TokenType
	}{
		{"+", token.TOKEN_PLUS},
		{"-", token.TOKEN_MINUS},
		{"*", token.TOKEN_STAR},
		{"/", token.TOKEN_SLASH},
		{"%", token.TOKEN_PERCENT},
		{">", token.TOKEN_GT},
		{"<", token.TOKEN_LT},
		{"!", token.TOKEN_NOT},
		{"=", token.TOKEN_ASSIGN},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			tokens, errs := New(tc.input).Tokenize()
			requireNoErrors(t, errs)
			if tokens[0].Type != tc.want {
				t.Errorf("got %s, want %s", tokens[0].Type, tc.want)
			}
		})
	}
}

func TestTwoCharOperators(t *testing.T) {
	cases := []struct {
		input string
		want  token.TokenType
	}{
		{">=", token.TOKEN_GTE},
		{"<=", token.TOKEN_LTE},
		{"==", token.TOKEN_EQ},
		{"!=", token.TOKEN_NEQ},
		{"&&", token.TOKEN_AND},
		{"||", token.TOKEN_OR},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			tokens, errs := New(tc.input).Tokenize()
			requireNoErrors(t, errs)
			if tokens[0].Type != tc.want {
				t.Errorf("got %s, want %s", tokens[0].Type, tc.want)
			}
			if tokens[0].Literal != tc.input {
				t.Errorf("literal: got %q, want %q", tokens[0].Literal, tc.input)
			}
		})
	}
}

func TestDelimiters(t *testing.T) {
	cases := []struct {
		input string
		want  token.TokenType
	}{
		{"(", token.TOKEN_LPAREN},
		{")", token.TOKEN_RPAREN},
		{"[", token.TOKEN_LBRACKET},
		{"]", token.TOKEN_RBRACKET},
		{"{", token.TOKEN_LBRACE},
		{"}", token.TOKEN_RBRACE},
		{":", token.TOKEN_COLON},
		{",", token.TOKEN_COMMA},
		{".", token.TOKEN_DOT},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			tokens, errs := New(tc.input).Tokenize()
			requireNoErrors(t, errs)
			if tokens[0].Type != tc.want {
				t.Errorf("got %s, want %s", tokens[0].Type, tc.want)
			}
		})
	}
}

func TestCommentSkipping(t *testing.T) {
	input := "SET x 5 # this is a comment\nLOG"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	// SET x 5 NEWLINE LOG EOF
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_SET, token.TOKEN_IDENT, token.TOKEN_INT,
		token.TOKEN_NEWLINE,
		token.TOKEN_LOG,
		token.TOKEN_EOF,
	})
}

func TestCommentOnlyLine(t *testing.T) {
	input := "# just a comment"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{token.TOKEN_EOF})
}

func TestNewlineCollapsing(t *testing.T) {
	input := "SET x 5\n\n\n\nLOG"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	// Multiple newlines collapse to one TOKEN_NEWLINE
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_SET, token.TOKEN_IDENT, token.TOKEN_INT,
		token.TOKEN_NEWLINE,
		token.TOKEN_LOG,
		token.TOKEN_EOF,
	})
}

func TestNewlineCollapsingWithWhitespace(t *testing.T) {
	input := "SET x\n  \n\t\n  \nLOG"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_SET, token.TOKEN_IDENT,
		token.TOKEN_NEWLINE,
		token.TOKEN_LOG,
		token.TOKEN_EOF,
	})
}

func TestPositionTracking(t *testing.T) {
	input := "SET x 5"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)

	// SET at line 1, col 1
	if tokens[0].Pos.Line != 1 || tokens[0].Pos.Column != 1 {
		t.Errorf("SET pos: got (%d,%d), want (1,1)", tokens[0].Pos.Line, tokens[0].Pos.Column)
	}
	// x at line 1, col 5
	if tokens[1].Pos.Line != 1 || tokens[1].Pos.Column != 5 {
		t.Errorf("x pos: got (%d,%d), want (1,5)", tokens[1].Pos.Line, tokens[1].Pos.Column)
	}
	// 5 at line 1, col 7
	if tokens[2].Pos.Line != 1 || tokens[2].Pos.Column != 7 {
		t.Errorf("5 pos: got (%d,%d), want (1,7)", tokens[2].Pos.Line, tokens[2].Pos.Column)
	}
}

func TestPositionMultiLine(t *testing.T) {
	input := "SET x 5\nLOG y"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	// SET x 5 NEWLINE LOG y EOF
	// LOG should be at line 2, col 1
	logTok := tokens[4] // SET(0) x(1) 5(2) NEWLINE(3) LOG(4)
	if logTok.Type != token.TOKEN_LOG {
		t.Fatalf("expected LOG at index 4, got %s", logTok.Type)
	}
	if logTok.Pos.Line != 2 || logTok.Pos.Column != 1 {
		t.Errorf("LOG pos: got (%d,%d), want (2,1)", logTok.Pos.Line, logTok.Pos.Column)
	}
	// y should be at line 2, col 5
	yTok := tokens[5]
	if yTok.Pos.Line != 2 || yTok.Pos.Column != 5 {
		t.Errorf("y pos: got (%d,%d), want (2,5)", yTok.Pos.Line, yTok.Pos.Column)
	}
}

func TestSetVoltageStatement(t *testing.T) {
	input := "SET voltage 5.0"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_SET, token.TOKEN_IDENT, token.TOKEN_FLOAT, token.TOKEN_EOF,
	})
	if tokens[1].Literal != "voltage" {
		t.Errorf("ident literal: got %q, want %q", tokens[1].Literal, "voltage")
	}
	if tokens[2].Literal != "5.0" {
		t.Errorf("float literal: got %q, want %q", tokens[2].Literal, "5.0")
	}
}

func TestComplexCondition(t *testing.T) {
	input := "IF voltage >= 4.9 && voltage <= 5.1"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_IF,
		token.TOKEN_IDENT, // voltage
		token.TOKEN_GTE,
		token.TOKEN_FLOAT, // 4.9
		token.TOKEN_AND,
		token.TOKEN_IDENT, // voltage
		token.TOKEN_LTE,
		token.TOKEN_FLOAT, // 5.1
		token.TOKEN_EOF,
	})
}

func TestArrayLiteral(t *testing.T) {
	input := "[1, 2, 3]"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_LBRACKET,
		token.TOKEN_INT, // 1
		token.TOKEN_COMMA,
		token.TOKEN_INT, // 2
		token.TOKEN_COMMA,
		token.TOKEN_INT, // 3
		token.TOKEN_RBRACKET,
		token.TOKEN_EOF,
	})
}

func TestDictLiteral(t *testing.T) {
	input := `{"key": "value"}`
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_LBRACE,
		token.TOKEN_STRING, // key
		token.TOKEN_COLON,
		token.TOKEN_STRING, // value
		token.TOKEN_RBRACE,
		token.TOKEN_EOF,
	})
	if tokens[1].Literal != "key" {
		t.Errorf("key literal: got %q, want %q", tokens[1].Literal, "key")
	}
	if tokens[3].Literal != "value" {
		t.Errorf("value literal: got %q, want %q", tokens[3].Literal, "value")
	}
}

func TestFunctionCall(t *testing.T) {
	input := "CALL measure(dmm, 101)"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_CALL,
		token.TOKEN_IDENT, // measure
		token.TOKEN_LPAREN,
		token.TOKEN_IDENT, // dmm
		token.TOKEN_COMMA,
		token.TOKEN_INT, // 101
		token.TOKEN_RPAREN,
		token.TOKEN_EOF,
	})
}

func TestBooleanLiterals(t *testing.T) {
	cases := []struct {
		input string
		want  token.TokenType
	}{
		{"true", token.TOKEN_TRUE},
		{"TRUE", token.TOKEN_TRUE},
		{"True", token.TOKEN_TRUE},
		{"false", token.TOKEN_FALSE},
		{"FALSE", token.TOKEN_FALSE},
		{"False", token.TOKEN_FALSE},
		{"null", token.TOKEN_NULL},
		{"NULL", token.TOKEN_NULL},
		{"Null", token.TOKEN_NULL},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			tokens, errs := New(tc.input).Tokenize()
			requireNoErrors(t, errs)
			if tokens[0].Type != tc.want {
				t.Errorf("got %s, want %s", tokens[0].Type, tc.want)
			}
		})
	}
}

func TestIdentifier(t *testing.T) {
	cases := []struct {
		input string
	}{
		{"voltage"},
		{"_private"},
		{"my_var_2"},
		{"camelCase"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			tokens, errs := New(tc.input).Tokenize()
			requireNoErrors(t, errs)
			if tokens[0].Type != token.TOKEN_IDENT {
				t.Errorf("got %s, want IDENT", tokens[0].Type)
			}
			if tokens[0].Literal != tc.input {
				t.Errorf("literal: got %q, want %q", tokens[0].Literal, tc.input)
			}
		})
	}
}

func TestIllegalCharacter(t *testing.T) {
	tokens, errs := New("@").Tokenize()
	if len(errs) == 0 {
		t.Fatal("expected a lex error for '@'")
	}
	if tokens[0].Type != token.TOKEN_ILLEGAL {
		t.Errorf("type: got %s, want ILLEGAL", tokens[0].Type)
	}
}

func TestFullScript(t *testing.T) {
	input := `# Simple test
TEST "Voltage Check"
    CONNECT dmm TCP "10.0.0.1:5025"
    SET voltage 5.0
    QUERY dmm "MEAS:VOLT?" result
    ASSERT result >= 4.9 && result <= 5.1
    DISCONNECT dmm
    PASS "OK"
ENDTEST`
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)

	// The comment on the first line is skipped but its trailing newline is
	// emitted, so the first token is NEWLINE followed by TEST.
	if tokens[0].Type != token.TOKEN_NEWLINE {
		t.Errorf("first token: got %s, want NEWLINE", tokens[0].Type)
	}
	if tokens[1].Type != token.TOKEN_TEST {
		t.Errorf("second token: got %s, want TEST", tokens[1].Type)
	}

	// Last token before EOF should close the script
	eofIdx := len(tokens) - 1
	if tokens[eofIdx].Type != token.TOKEN_EOF {
		t.Errorf("last token: got %s, want EOF", tokens[eofIdx].Type)
	}

	// Find ENDTEST
	found := false
	for _, tok := range tokens {
		if tok.Type == token.TOKEN_ENDTEST {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ENDTEST token in output")
	}
}

func TestOperatorPrecedenceTokens(t *testing.T) {
	// Ensure >= doesn't get split into > and =
	input := "a >= b <= c == d != e"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_IDENT, // a
		token.TOKEN_GTE,
		token.TOKEN_IDENT, // b
		token.TOKEN_LTE,
		token.TOKEN_IDENT, // c
		token.TOKEN_EQ,
		token.TOKEN_IDENT, // d
		token.TOKEN_NEQ,
		token.TOKEN_IDENT, // e
		token.TOKEN_EOF,
	})
}

func TestNotOperator(t *testing.T) {
	input := "!enabled"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_NOT,
		token.TOKEN_IDENT,
		token.TOKEN_EOF,
	})
}

func TestMultipleStatements(t *testing.T) {
	input := "SET a 1\nSET b 2\nSET c 3"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_SET, token.TOKEN_IDENT, token.TOKEN_INT,
		token.TOKEN_NEWLINE,
		token.TOKEN_SET, token.TOKEN_IDENT, token.TOKEN_INT,
		token.TOKEN_NEWLINE,
		token.TOKEN_SET, token.TOKEN_IDENT, token.TOKEN_INT,
		token.TOKEN_EOF,
	})
}

func TestTrailingNewline(t *testing.T) {
	input := "SET x 1\n"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	// Trailing newline is consumed and collapsed; no extra token after EOF
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_SET, token.TOKEN_IDENT, token.TOKEN_INT,
		token.TOKEN_NEWLINE,
		token.TOKEN_EOF,
	})
}

func TestLeadingNewlines(t *testing.T) {
	input := "\n\n\nSET x 1"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	// Leading newlines produce one NEWLINE token before the statement
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_NEWLINE,
		token.TOKEN_SET, token.TOKEN_IDENT, token.TOKEN_INT,
		token.TOKEN_EOF,
	})
}

func TestArithmeticExpression(t *testing.T) {
	input := "a + b * c - d / e % f"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_IDENT, token.TOKEN_PLUS,
		token.TOKEN_IDENT, token.TOKEN_STAR,
		token.TOKEN_IDENT, token.TOKEN_MINUS,
		token.TOKEN_IDENT, token.TOKEN_SLASH,
		token.TOKEN_IDENT, token.TOKEN_PERCENT,
		token.TOKEN_IDENT,
		token.TOKEN_EOF,
	})
}

func TestTokenTypeString(t *testing.T) {
	cases := []struct {
		tt   token.TokenType
		want string
	}{
		{token.TOKEN_EOF, "EOF"},
		{token.TOKEN_SET, "SET"},
		{token.TOKEN_PLUS, "+"},
		{token.TOKEN_GTE, ">="},
		{token.TOKEN_AND, "&&"},
		{token.TOKEN_LPAREN, "("},
		{token.TOKEN_STRING, "STRING"},
		{token.TOKEN_TRUE, "TRUE"},
		{token.TOKEN_NULL, "NULL"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			got := tc.tt.String()
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAssignVsEquals(t *testing.T) {
	// Single = is ASSIGN, double == is EQ
	input := "a = b == c"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_IDENT,
		token.TOKEN_ASSIGN,
		token.TOKEN_IDENT,
		token.TOKEN_EQ,
		token.TOKEN_IDENT,
		token.TOKEN_EOF,
	})
}

func TestDotInNonNumber(t *testing.T) {
	// A dot not preceded by digits should be TOKEN_DOT
	input := "obj.field"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_IDENT, // obj
		token.TOKEN_DOT,
		token.TOKEN_IDENT, // field
		token.TOKEN_EOF,
	})
}

func TestCommentAtEndOfFile(t *testing.T) {
	input := "SET x 1 # end comment"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_SET, token.TOKEN_IDENT, token.TOKEN_INT,
		token.TOKEN_EOF,
	})
}

func TestNewlineCollapsingWithComments(t *testing.T) {
	input := "SET x 1\n# comment\n\nSET y 2"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_SET, token.TOKEN_IDENT, token.TOKEN_INT,
		token.TOKEN_NEWLINE,
		token.TOKEN_SET, token.TOKEN_IDENT, token.TOKEN_INT,
		token.TOKEN_EOF,
	})
}

func TestLexErrorDetails(t *testing.T) {
	input := `"unterminated`
	_, errs := New(input).Tokenize()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Line != 1 {
		t.Errorf("error line: got %d, want 1", errs[0].Line)
	}
	if errs[0].Column != 1 {
		t.Errorf("error col: got %d, want 1", errs[0].Column)
	}
}

func TestLexErrorString(t *testing.T) {
	e := LexError{Line: 3, Column: 7, Message: "bad token"}
	got := e.Error()
	want := "line 3, column 7: bad token"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMixedNumbersAndDots(t *testing.T) {
	// 5.0 is a float but 5. followed by non-digit is int + dot
	input := "5.0"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	if tokens[0].Type != token.TOKEN_FLOAT {
		t.Errorf("got %s, want FLOAT", tokens[0].Type)
	}

	// 5 followed by .x  =>  INT DOT IDENT
	input2 := "5.x"
	tokens2, errs2 := New(input2).Tokenize()
	requireNoErrors(t, errs2)
	requireTypes(t, tokens2, []token.TokenType{
		token.TOKEN_INT, token.TOKEN_DOT, token.TOKEN_IDENT, token.TOKEN_EOF,
	})
}

func TestParallelTimeout(t *testing.T) {
	input := "PARALLEL TIMEOUT 10000"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_PARALLEL, token.TOKEN_TIMEOUT, token.TOKEN_INT, token.TOKEN_EOF,
	})
}

func TestForeachLoop(t *testing.T) {
	input := "FOREACH item IN items"
	tokens, errs := New(input).Tokenize()
	requireNoErrors(t, errs)
	requireTypes(t, tokens, []token.TokenType{
		token.TOKEN_FOREACH, token.TOKEN_IDENT, token.TOKEN_IN, token.TOKEN_IDENT, token.TOKEN_EOF,
	})
}

func TestMultipleErrorRecovery(t *testing.T) {
	// Two illegal characters should produce two errors
	input := "@ $"
	tokens, errs := New(input).Tokenize()
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errs))
	}
	// Should still have tokens (ILLEGAL, ILLEGAL, EOF)
	illegalCount := 0
	for _, tok := range tokens {
		if tok.Type == token.TOKEN_ILLEGAL {
			illegalCount++
		}
	}
	if illegalCount != 2 {
		t.Errorf("expected 2 ILLEGAL tokens, got %d", illegalCount)
	}
}

func TestKeywordLookupFunc(t *testing.T) {
	cases := []struct {
		ident string
		want  token.TokenType
	}{
		{"SET", token.TOKEN_SET},
		{"set", token.TOKEN_SET},
		{"SeTuP", token.TOKEN_SETUP},
		{"notakeyword", token.TOKEN_IDENT},
		{"voltage", token.TOKEN_IDENT},
		{"TRUE", token.TOKEN_TRUE},
		{"false", token.TOKEN_FALSE},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("KeywordLookup(%s)", tc.ident), func(t *testing.T) {
			got := token.KeywordLookup(tc.ident)
			if got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}
