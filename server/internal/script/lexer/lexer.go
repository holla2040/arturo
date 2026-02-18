package lexer

import (
	"fmt"
	"unicode"

	"github.com/holla2040/arturo/internal/script/token"
)

// LexError records a lexing error at a specific position.
type LexError struct {
	Line    int
	Column  int
	Message string
}

// Error implements the error interface.
func (e LexError) Error() string {
	return fmt.Errorf("line %d, column %d: %s", e.Line, e.Column, e.Message).Error()
}

// Lexer scans source text into tokens.
type Lexer struct {
	source []rune
	pos    int // current position in source (index into rune slice)
	line   int // current line number (1-based)
	col    int // current column number (1-based)
	tokens []token.Token
	errors []LexError
}

// New creates a Lexer for the given source string.
func New(source string) *Lexer {
	return &Lexer{
		source: []rune(source),
		pos:    0,
		line:   1,
		col:    1,
	}
}

// Tokenize scans the entire source and returns the resulting tokens and any
// lexing errors. The token slice always ends with TOKEN_EOF.
func (l *Lexer) Tokenize() ([]token.Token, []LexError) {
	for {
		l.skipWhitespace()

		if l.atEnd() {
			l.emit(token.TOKEN_EOF, "")
			break
		}

		ch := l.peek()

		switch {
		case ch == '#':
			l.skipComment()

		case ch == '\n':
			l.scanNewlines()

		case ch == '\r':
			// Handle \r\n or bare \r as a newline
			l.scanNewlines()

		case isIdentStart(ch):
			l.scanIdentifier()

		case isDigit(ch):
			l.scanNumber()

		case ch == '"':
			l.scanString()

		default:
			l.scanOperatorOrDelimiter()
		}
	}

	return l.tokens, l.errors
}

// ---------------------------------------------------------------------------
// Character helpers
// ---------------------------------------------------------------------------

func (l *Lexer) atEnd() bool {
	return l.pos >= len(l.source)
}

func (l *Lexer) peek() rune {
	if l.atEnd() {
		return 0
	}
	return l.source[l.pos]
}

func (l *Lexer) peekAt(offset int) rune {
	idx := l.pos + offset
	if idx >= len(l.source) {
		return 0
	}
	return l.source[idx]
}

// advance consumes one rune and updates position tracking.
func (l *Lexer) advance() rune {
	ch := l.source[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

// emit appends a token with the current position (before the token literal).
func (l *Lexer) emit(tt token.TokenType, literal string) {
	// Position is already set by the caller before consuming; we use the
	// saved start position passed through emitAt.
	// This method is a convenience that should only be called when the
	// position was captured beforehand. We'll store at current pos.
	l.tokens = append(l.tokens, token.Token{
		Type:    tt,
		Literal: literal,
		Pos:     token.Position{Line: l.line, Column: l.col, Offset: l.pos},
	})
}

// emitAt appends a token with an explicit position.
func (l *Lexer) emitAt(tt token.TokenType, literal string, pos token.Position) {
	l.tokens = append(l.tokens, token.Token{
		Type:    tt,
		Literal: literal,
		Pos:     pos,
	})
}

// addError records a lex error.
func (l *Lexer) addError(line, col int, msg string) {
	l.errors = append(l.errors, LexError{Line: line, Column: col, Message: msg})
}

// savePos captures the current position for a token that is about to be scanned.
func (l *Lexer) savePos() token.Position {
	return token.Position{Line: l.line, Column: l.col, Offset: l.pos}
}

// ---------------------------------------------------------------------------
// Whitespace & comments
// ---------------------------------------------------------------------------

func (l *Lexer) skipWhitespace() {
	for !l.atEnd() {
		ch := l.peek()
		if ch == ' ' || ch == '\t' {
			l.advance()
		} else {
			break
		}
	}
}

func (l *Lexer) skipComment() {
	// '#' starts a comment; skip until newline (do not consume the newline).
	for !l.atEnd() && l.peek() != '\n' && l.peek() != '\r' {
		l.advance()
	}
}

// ---------------------------------------------------------------------------
// Newlines (collapse consecutive)
// ---------------------------------------------------------------------------

func (l *Lexer) scanNewlines() {
	pos := l.savePos()
	// Consume the first newline.
	l.consumeNewline()

	// Collapse consecutive blank lines (whitespace-only lines count).
	for !l.atEnd() {
		ch := l.peek()
		if ch == ' ' || ch == '\t' {
			l.advance()
			continue
		}
		if ch == '\n' || ch == '\r' {
			l.consumeNewline()
			continue
		}
		if ch == '#' {
			// A comment on an otherwise-blank line is still a blank line.
			l.skipComment()
			continue
		}
		break
	}

	l.emitAt(token.TOKEN_NEWLINE, "\n", pos)
}

// consumeNewline eats one \n or \r\n.
func (l *Lexer) consumeNewline() {
	if l.peek() == '\r' {
		l.advance()
		if !l.atEnd() && l.peek() == '\n' {
			l.advance()
		}
	} else {
		l.advance() // '\n'
	}
}

// ---------------------------------------------------------------------------
// Identifiers and keywords
// ---------------------------------------------------------------------------

func isIdentStart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch)
}

func isIdentPart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch)
}

func (l *Lexer) scanIdentifier() {
	pos := l.savePos()
	start := l.pos
	for !l.atEnd() && isIdentPart(l.peek()) {
		l.advance()
	}
	literal := string(l.source[start:l.pos])
	tt := token.KeywordLookup(literal)
	l.emitAt(tt, literal, pos)
}

// ---------------------------------------------------------------------------
// Numbers (int, float, scientific notation)
// ---------------------------------------------------------------------------

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func (l *Lexer) scanNumber() {
	pos := l.savePos()
	start := l.pos
	isFloat := false

	// Integer part
	for !l.atEnd() && isDigit(l.peek()) {
		l.advance()
	}

	// Fractional part
	if !l.atEnd() && l.peek() == '.' && isDigit(l.peekAt(1)) {
		isFloat = true
		l.advance() // consume '.'
		for !l.atEnd() && isDigit(l.peek()) {
			l.advance()
		}
	}

	// Exponent part (e.g. e+3, E-2, e10)
	if !l.atEnd() && (l.peek() == 'e' || l.peek() == 'E') {
		isFloat = true
		l.advance() // consume 'e'/'E'
		if !l.atEnd() && (l.peek() == '+' || l.peek() == '-') {
			l.advance()
		}
		if l.atEnd() || !isDigit(l.peek()) {
			literal := string(l.source[start:l.pos])
			l.addError(pos.Line, pos.Column, fmt.Sprintf("invalid number literal: %s", literal))
			l.emitAt(token.TOKEN_ILLEGAL, literal, pos)
			return
		}
		for !l.atEnd() && isDigit(l.peek()) {
			l.advance()
		}
	}

	literal := string(l.source[start:l.pos])
	if isFloat {
		l.emitAt(token.TOKEN_FLOAT, literal, pos)
	} else {
		l.emitAt(token.TOKEN_INT, literal, pos)
	}
}

// ---------------------------------------------------------------------------
// Strings (double-quoted with escape sequences)
// ---------------------------------------------------------------------------

func (l *Lexer) scanString() {
	pos := l.savePos()
	l.advance() // consume opening '"'
	var buf []rune

	for {
		if l.atEnd() {
			l.addError(pos.Line, pos.Column, "unterminated string literal")
			l.emitAt(token.TOKEN_ILLEGAL, string(buf), pos)
			return
		}
		ch := l.peek()
		if ch == '\n' || ch == '\r' {
			l.addError(pos.Line, pos.Column, "unterminated string literal")
			l.emitAt(token.TOKEN_ILLEGAL, string(buf), pos)
			return
		}
		if ch == '"' {
			l.advance() // consume closing '"'
			l.emitAt(token.TOKEN_STRING, string(buf), pos)
			return
		}
		if ch == '\\' {
			l.advance() // consume backslash
			if l.atEnd() {
				l.addError(pos.Line, pos.Column, "unterminated string literal")
				l.emitAt(token.TOKEN_ILLEGAL, string(buf), pos)
				return
			}
			esc := l.advance()
			switch esc {
			case '"':
				buf = append(buf, '"')
			case '\\':
				buf = append(buf, '\\')
			case 'n':
				buf = append(buf, '\n')
			case 't':
				buf = append(buf, '\t')
			default:
				// Keep unrecognised escapes as-is (backslash + char).
				buf = append(buf, '\\', esc)
			}
			continue
		}
		buf = append(buf, l.advance())
	}
}

// ---------------------------------------------------------------------------
// Operators and delimiters
// ---------------------------------------------------------------------------

func (l *Lexer) scanOperatorOrDelimiter() {
	pos := l.savePos()
	ch := l.peek()
	next := l.peekAt(1)

	// Two-character operators (check first)
	switch {
	case ch == '>' && next == '=':
		l.advance()
		l.advance()
		l.emitAt(token.TOKEN_GTE, ">=", pos)
		return
	case ch == '<' && next == '=':
		l.advance()
		l.advance()
		l.emitAt(token.TOKEN_LTE, "<=", pos)
		return
	case ch == '=' && next == '=':
		l.advance()
		l.advance()
		l.emitAt(token.TOKEN_EQ, "==", pos)
		return
	case ch == '!' && next == '=':
		l.advance()
		l.advance()
		l.emitAt(token.TOKEN_NEQ, "!=", pos)
		return
	case ch == '&' && next == '&':
		l.advance()
		l.advance()
		l.emitAt(token.TOKEN_AND, "&&", pos)
		return
	case ch == '|' && next == '|':
		l.advance()
		l.advance()
		l.emitAt(token.TOKEN_OR, "||", pos)
		return
	}

	// Single-character operators and delimiters
	l.advance()
	switch ch {
	case '+':
		l.emitAt(token.TOKEN_PLUS, "+", pos)
	case '-':
		l.emitAt(token.TOKEN_MINUS, "-", pos)
	case '*':
		l.emitAt(token.TOKEN_STAR, "*", pos)
	case '/':
		l.emitAt(token.TOKEN_SLASH, "/", pos)
	case '%':
		l.emitAt(token.TOKEN_PERCENT, "%", pos)
	case '>':
		l.emitAt(token.TOKEN_GT, ">", pos)
	case '<':
		l.emitAt(token.TOKEN_LT, "<", pos)
	case '!':
		l.emitAt(token.TOKEN_NOT, "!", pos)
	case '=':
		l.emitAt(token.TOKEN_ASSIGN, "=", pos)
	case '(':
		l.emitAt(token.TOKEN_LPAREN, "(", pos)
	case ')':
		l.emitAt(token.TOKEN_RPAREN, ")", pos)
	case '[':
		l.emitAt(token.TOKEN_LBRACKET, "[", pos)
	case ']':
		l.emitAt(token.TOKEN_RBRACKET, "]", pos)
	case '{':
		l.emitAt(token.TOKEN_LBRACE, "{", pos)
	case '}':
		l.emitAt(token.TOKEN_RBRACE, "}", pos)
	case ':':
		l.emitAt(token.TOKEN_COLON, ":", pos)
	case ',':
		l.emitAt(token.TOKEN_COMMA, ",", pos)
	case '.':
		l.emitAt(token.TOKEN_DOT, ".", pos)
	default:
		l.addError(pos.Line, pos.Column, fmt.Sprintf("unexpected character: %c", ch))
		l.emitAt(token.TOKEN_ILLEGAL, string(ch), pos)
	}
}
