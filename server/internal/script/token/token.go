package token

import "strings"

// TokenType represents the type of a lexical token.
type TokenType int

const (
	// Special tokens
	TOKEN_EOF     TokenType = iota
	TOKEN_NEWLINE           // significant newline (statement separator)
	TOKEN_ILLEGAL           // unrecognized character

	// Identifiers and literals
	TOKEN_IDENT  // user-defined identifier
	TOKEN_INT    // integer literal
	TOKEN_FLOAT  // floating-point literal (includes scientific notation)
	TOKEN_STRING // string literal (double-quoted)

	// ----- Keywords (ALL CAPS in the language, lookup is case-insensitive) -----

	// Test structure
	TOKEN_TEST
	TOKEN_ENDTEST
	TOKEN_SUITE
	TOKEN_ENDSUITE
	TOKEN_SETUP
	TOKEN_ENDSETUP
	TOKEN_TEARDOWN
	TOKEN_ENDTEARDOWN

	// Variable commands
	TOKEN_SET
	TOKEN_CONST
	TOKEN_GLOBAL
	TOKEN_DELETE
	TOKEN_APPEND
	TOKEN_EXTEND

	// Conditionals
	TOKEN_IF
	TOKEN_ELSEIF
	TOKEN_ELSE
	TOKEN_ENDIF

	// Loops
	TOKEN_LOOP
	TOKEN_TIMES
	TOKEN_AS
	TOKEN_ENDLOOP
	TOKEN_WHILE
	TOKEN_ENDWHILE
	TOKEN_FOREACH
	TOKEN_IN
	TOKEN_ENDFOREACH

	// Loop control
	TOKEN_BREAK
	TOKEN_CONTINUE

	// Error handling
	TOKEN_TRY
	TOKEN_CATCH
	TOKEN_FINALLY
	TOKEN_ENDTRY

	// Parallel execution
	TOKEN_PARALLEL
	TOKEN_ENDPARALLEL
	TOKEN_TIMEOUT

	// Device communication
	TOKEN_CONNECT
	TOKEN_DISCONNECT
	TOKEN_TCP
	TOKEN_SERIAL
	TOKEN_ALL

	// Device commands
	TOKEN_SEND
	TOKEN_QUERY
	TOKEN_RELAY

	// Functions
	TOKEN_FUNCTION
	TOKEN_ENDFUNCTION
	TOKEN_CALL
	TOKEN_RETURN

	// Libraries
	TOKEN_IMPORT
	TOKEN_LIBRARY
	TOKEN_ENDLIBRARY

	// Test results and utilities
	TOKEN_PASS
	TOKEN_FAIL
	TOKEN_SKIP
	TOKEN_ASSERT
	TOKEN_LOG
	TOKEN_DELAY

	// Misc keywords
	TOKEN_RESERVE
	TOKEN_ON
	TOKEN_OFF
	TOKEN_TOGGLE
	TOKEN_GET

	// Boolean and null literals
	TOKEN_TRUE
	TOKEN_FALSE
	TOKEN_NULL

	// ----- Operators -----
	TOKEN_PLUS    // +
	TOKEN_MINUS   // -
	TOKEN_STAR    // *
	TOKEN_SLASH   // /
	TOKEN_PERCENT // %

	TOKEN_GT  // >
	TOKEN_LT  // <
	TOKEN_GTE // >=
	TOKEN_LTE // <=
	TOKEN_EQ  // ==
	TOKEN_NEQ // !=

	TOKEN_AND    // &&
	TOKEN_OR     // ||
	TOKEN_NOT    // !
	TOKEN_ASSIGN // =

	// ----- Delimiters -----
	TOKEN_LPAREN   // (
	TOKEN_RPAREN   // )
	TOKEN_LBRACKET // [
	TOKEN_RBRACKET // ]
	TOKEN_LBRACE   // {
	TOKEN_RBRACE   // }
	TOKEN_COLON    // :
	TOKEN_COMMA    // ,
	TOKEN_DOT      // .
)

// Position records where a token was found in the source text.
type Position struct {
	Line   int // 1-based line number
	Column int // 1-based column number
	Offset int // 0-based byte offset into source
}

// Token is a single lexical token produced by the lexer.
type Token struct {
	Type    TokenType
	Literal string
	Pos     Position
}

// keywords maps upper-cased keyword strings to their token types.
var keywords = map[string]TokenType{
	"TEST":         TOKEN_TEST,
	"ENDTEST":      TOKEN_ENDTEST,
	"SUITE":        TOKEN_SUITE,
	"ENDSUITE":     TOKEN_ENDSUITE,
	"SETUP":        TOKEN_SETUP,
	"ENDSETUP":     TOKEN_ENDSETUP,
	"TEARDOWN":     TOKEN_TEARDOWN,
	"ENDTEARDOWN":  TOKEN_ENDTEARDOWN,
	"SET":          TOKEN_SET,
	"CONST":        TOKEN_CONST,
	"GLOBAL":       TOKEN_GLOBAL,
	"DELETE":       TOKEN_DELETE,
	"APPEND":       TOKEN_APPEND,
	"EXTEND":       TOKEN_EXTEND,
	"IF":           TOKEN_IF,
	"ELSEIF":       TOKEN_ELSEIF,
	"ELSE":         TOKEN_ELSE,
	"ENDIF":        TOKEN_ENDIF,
	"LOOP":         TOKEN_LOOP,
	"TIMES":        TOKEN_TIMES,
	"AS":           TOKEN_AS,
	"ENDLOOP":      TOKEN_ENDLOOP,
	"WHILE":        TOKEN_WHILE,
	"ENDWHILE":     TOKEN_ENDWHILE,
	"FOREACH":      TOKEN_FOREACH,
	"IN":           TOKEN_IN,
	"ENDFOREACH":   TOKEN_ENDFOREACH,
	"BREAK":        TOKEN_BREAK,
	"CONTINUE":     TOKEN_CONTINUE,
	"TRY":          TOKEN_TRY,
	"CATCH":        TOKEN_CATCH,
	"FINALLY":      TOKEN_FINALLY,
	"ENDTRY":       TOKEN_ENDTRY,
	"PARALLEL":     TOKEN_PARALLEL,
	"ENDPARALLEL":  TOKEN_ENDPARALLEL,
	"TIMEOUT":      TOKEN_TIMEOUT,
	"CONNECT":      TOKEN_CONNECT,
	"DISCONNECT":   TOKEN_DISCONNECT,
	"TCP":          TOKEN_TCP,
	"SERIAL":       TOKEN_SERIAL,
	"ALL":          TOKEN_ALL,
	"SEND":         TOKEN_SEND,
	"QUERY":        TOKEN_QUERY,
	"RELAY":        TOKEN_RELAY,
	"FUNCTION":     TOKEN_FUNCTION,
	"ENDFUNCTION":  TOKEN_ENDFUNCTION,
	"CALL":         TOKEN_CALL,
	"RETURN":       TOKEN_RETURN,
	"IMPORT":       TOKEN_IMPORT,
	"LIBRARY":      TOKEN_LIBRARY,
	"ENDLIBRARY":   TOKEN_ENDLIBRARY,
	"PASS":         TOKEN_PASS,
	"FAIL":         TOKEN_FAIL,
	"SKIP":         TOKEN_SKIP,
	"ASSERT":       TOKEN_ASSERT,
	"LOG":          TOKEN_LOG,
	"DELAY":        TOKEN_DELAY,
	"RESERVE":      TOKEN_RESERVE,
	"ON":           TOKEN_ON,
	"OFF":          TOKEN_OFF,
	"TOGGLE":       TOKEN_TOGGLE,
	"GET":          TOKEN_GET,
	"TRUE":         TOKEN_TRUE,
	"FALSE":        TOKEN_FALSE,
	"NULL":         TOKEN_NULL,
}

// KeywordLookup returns the keyword TokenType for ident (case-insensitive),
// or TOKEN_IDENT if it is not a keyword.
func KeywordLookup(ident string) TokenType {
	if tt, ok := keywords[strings.ToUpper(ident)]; ok {
		return tt
	}
	return TOKEN_IDENT
}

// tokenNames gives a human-readable name for each TokenType.
var tokenNames = map[TokenType]string{
	TOKEN_EOF:     "EOF",
	TOKEN_NEWLINE: "NEWLINE",
	TOKEN_ILLEGAL: "ILLEGAL",

	TOKEN_IDENT:  "IDENT",
	TOKEN_INT:    "INT",
	TOKEN_FLOAT:  "FLOAT",
	TOKEN_STRING: "STRING",

	TOKEN_TEST:        "TEST",
	TOKEN_ENDTEST:     "ENDTEST",
	TOKEN_SUITE:       "SUITE",
	TOKEN_ENDSUITE:    "ENDSUITE",
	TOKEN_SETUP:       "SETUP",
	TOKEN_ENDSETUP:    "ENDSETUP",
	TOKEN_TEARDOWN:    "TEARDOWN",
	TOKEN_ENDTEARDOWN: "ENDTEARDOWN",

	TOKEN_SET:    "SET",
	TOKEN_CONST:  "CONST",
	TOKEN_GLOBAL: "GLOBAL",
	TOKEN_DELETE: "DELETE",
	TOKEN_APPEND: "APPEND",
	TOKEN_EXTEND: "EXTEND",

	TOKEN_IF:     "IF",
	TOKEN_ELSEIF: "ELSEIF",
	TOKEN_ELSE:   "ELSE",
	TOKEN_ENDIF:  "ENDIF",

	TOKEN_LOOP:       "LOOP",
	TOKEN_TIMES:      "TIMES",
	TOKEN_AS:         "AS",
	TOKEN_ENDLOOP:    "ENDLOOP",
	TOKEN_WHILE:      "WHILE",
	TOKEN_ENDWHILE:   "ENDWHILE",
	TOKEN_FOREACH:    "FOREACH",
	TOKEN_IN:         "IN",
	TOKEN_ENDFOREACH: "ENDFOREACH",

	TOKEN_BREAK:    "BREAK",
	TOKEN_CONTINUE: "CONTINUE",

	TOKEN_TRY:     "TRY",
	TOKEN_CATCH:   "CATCH",
	TOKEN_FINALLY: "FINALLY",
	TOKEN_ENDTRY:  "ENDTRY",

	TOKEN_PARALLEL:    "PARALLEL",
	TOKEN_ENDPARALLEL: "ENDPARALLEL",
	TOKEN_TIMEOUT:     "TIMEOUT",

	TOKEN_CONNECT:    "CONNECT",
	TOKEN_DISCONNECT: "DISCONNECT",
	TOKEN_TCP:        "TCP",
	TOKEN_SERIAL:     "SERIAL",
	TOKEN_ALL:        "ALL",

	TOKEN_SEND:  "SEND",
	TOKEN_QUERY: "QUERY",
	TOKEN_RELAY: "RELAY",

	TOKEN_FUNCTION:    "FUNCTION",
	TOKEN_ENDFUNCTION: "ENDFUNCTION",
	TOKEN_CALL:        "CALL",
	TOKEN_RETURN:      "RETURN",

	TOKEN_IMPORT:     "IMPORT",
	TOKEN_LIBRARY:    "LIBRARY",
	TOKEN_ENDLIBRARY: "ENDLIBRARY",

	TOKEN_PASS:   "PASS",
	TOKEN_FAIL:   "FAIL",
	TOKEN_SKIP:   "SKIP",
	TOKEN_ASSERT: "ASSERT",
	TOKEN_LOG:    "LOG",
	TOKEN_DELAY:  "DELAY",

	TOKEN_RESERVE: "RESERVE",
	TOKEN_ON:      "ON",
	TOKEN_OFF:     "OFF",
	TOKEN_TOGGLE:  "TOGGLE",
	TOKEN_GET:     "GET",

	TOKEN_TRUE:  "TRUE",
	TOKEN_FALSE: "FALSE",
	TOKEN_NULL:  "NULL",

	TOKEN_PLUS:    "+",
	TOKEN_MINUS:   "-",
	TOKEN_STAR:    "*",
	TOKEN_SLASH:   "/",
	TOKEN_PERCENT: "%",

	TOKEN_GT:  ">",
	TOKEN_LT:  "<",
	TOKEN_GTE: ">=",
	TOKEN_LTE: "<=",
	TOKEN_EQ:  "==",
	TOKEN_NEQ: "!=",

	TOKEN_AND:    "&&",
	TOKEN_OR:     "||",
	TOKEN_NOT:    "!",
	TOKEN_ASSIGN: "=",

	TOKEN_LPAREN:   "(",
	TOKEN_RPAREN:   ")",
	TOKEN_LBRACKET: "[",
	TOKEN_RBRACKET: "]",
	TOKEN_LBRACE:   "{",
	TOKEN_RBRACE:   "}",
	TOKEN_COLON:    ":",
	TOKEN_COMMA:    ",",
	TOKEN_DOT:      ".",
}

// String returns a human-readable name for the token type.
func (t TokenType) String() string {
	if name, ok := tokenNames[t]; ok {
		return name
	}
	return "UNKNOWN"
}
