// Package parser implements a recursive descent parser for the Arturo
// scripting language (.art files). It consumes a token slice produced by
// the lexer and builds an AST.
package parser

import (
	"fmt"
	"strings"

	"github.com/holla2040/arturo/internal/script/ast"
	"github.com/holla2040/arturo/internal/script/token"
)

// ---------------------------------------------------------------------------
// ParseError
// ---------------------------------------------------------------------------

// ParseError records a single error encountered during parsing.
type ParseError struct {
	Line     int
	Column   int
	Severity string // "error" or "warning"
	Message  string
	Context  string // source line context if available
}

// Error implements the error interface.
func (e ParseError) Error() string {
	return fmt.Sprintf("line %d, column %d: %s: %s", e.Line, e.Column, e.Severity, e.Message)
}

// ---------------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------------

// Parser converts a token stream into an AST.
type Parser struct {
	tokens []token.Token
	pos    int
	errors []ParseError
}

// New creates a Parser for the given token slice (must end with TOKEN_EOF).
func New(tokens []token.Token) *Parser {
	return &Parser{
		tokens: tokens,
		pos:    0,
	}
}

// Parse runs the parser and returns the resulting program AST together with
// any errors that were encountered.
func (p *Parser) Parse() (*ast.Program, []ParseError) {
	prog := &ast.Program{}
	if len(p.tokens) > 0 {
		prog.Position = p.tokens[0].Pos
	}

	p.skipNewlines()

	for !p.atEnd() {
		stmt := p.parseStatement()
		if stmt != nil {
			prog.Statements = append(prog.Statements, stmt)
		}
		p.skipNewlines()
	}

	return prog, p.errors
}

// ---------------------------------------------------------------------------
// Token navigation
// ---------------------------------------------------------------------------

func (p *Parser) peek() token.Token {
	if p.pos >= len(p.tokens) {
		return token.Token{Type: token.TOKEN_EOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) peekType() token.TokenType {
	return p.peek().Type
}

// peekAt returns the token at the given offset from the current position.
func (p *Parser) peekAt(offset int) token.Token {
	idx := p.pos + offset
	if idx >= len(p.tokens) {
		return token.Token{Type: token.TOKEN_EOF}
	}
	return p.tokens[idx]
}

func (p *Parser) advance() token.Token {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

func (p *Parser) expect(tt token.TokenType) token.Token {
	tok := p.peek()
	if tok.Type == tt {
		return p.advance()
	}
	p.addError(tok.Pos, fmt.Sprintf("expected %s, got %s", tt, tok.Type))
	return tok
}

// expectDeviceID consumes an IDENT or STRING token and returns its literal.
// Device IDs like "PUMP-01" contain hyphens that the lexer cannot represent
// as a single identifier, so we accept a quoted string as well.
func (p *Parser) expectDeviceID() string {
	tok := p.peek()
	if tok.Type == token.TOKEN_IDENT || tok.Type == token.TOKEN_STRING {
		p.advance()
		return tok.Literal
	}
	p.addError(tok.Pos, fmt.Sprintf("expected device ID (IDENT or STRING), got %s", tok.Type))
	return tok.Literal
}

func (p *Parser) match(types ...token.TokenType) bool {
	for _, tt := range types {
		if p.peekType() == tt {
			p.advance()
			return true
		}
	}
	return false
}

func (p *Parser) skipNewlines() {
	for p.peekType() == token.TOKEN_NEWLINE {
		p.advance()
	}
}

func (p *Parser) atEnd() bool {
	return p.peekType() == token.TOKEN_EOF
}

func (p *Parser) addError(pos token.Position, msg string) {
	p.errors = append(p.errors, ParseError{
		Line:     pos.Line,
		Column:   pos.Column,
		Severity: "error",
		Message:  msg,
	})
}

// synchronize skips tokens until we reach a newline or a known
// statement-starting keyword, enabling multi-error reporting.
func (p *Parser) synchronize() {
	for !p.atEnd() {
		if p.peekType() == token.TOKEN_NEWLINE {
			p.advance()
			return
		}
		if isStatementStart(p.peekType()) {
			return
		}
		p.advance()
	}
}

// isStatementStart returns true if tt could begin a statement.
func isStatementStart(tt token.TokenType) bool {
	switch tt {
	case token.TOKEN_SET, token.TOKEN_CONST, token.TOKEN_GLOBAL,
		token.TOKEN_DELETE, token.TOKEN_APPEND, token.TOKEN_EXTEND,
		token.TOKEN_IF, token.TOKEN_LOOP, token.TOKEN_WHILE, token.TOKEN_FOREACH,
		token.TOKEN_BREAK, token.TOKEN_CONTINUE,
		token.TOKEN_TRY, token.TOKEN_PARALLEL,
		token.TOKEN_CONNECT, token.TOKEN_DISCONNECT,
		token.TOKEN_SEND, token.TOKEN_QUERY, token.TOKEN_RELAY,
		token.TOKEN_FUNCTION, token.TOKEN_CALL, token.TOKEN_RETURN,
		token.TOKEN_IMPORT, token.TOKEN_LIBRARY,
		token.TOKEN_PASS, token.TOKEN_FAIL, token.TOKEN_SKIP,
		token.TOKEN_ASSERT, token.TOKEN_LOG, token.TOKEN_DELAY,
		token.TOKEN_RESERVE,
		token.TOKEN_TEST, token.TOKEN_SUITE:
		return true
	}
	return false
}

// isBlockEnd returns true if tt would end a block (any END* keyword, or
// ELSEIF/ELSE/CATCH/FINALLY which transition within a block).
func isBlockEnd(tt token.TokenType) bool {
	switch tt {
	case token.TOKEN_ENDTEST, token.TOKEN_ENDSUITE,
		token.TOKEN_ENDSETUP, token.TOKEN_ENDTEARDOWN,
		token.TOKEN_ENDIF, token.TOKEN_ELSEIF, token.TOKEN_ELSE,
		token.TOKEN_ENDLOOP, token.TOKEN_ENDWHILE, token.TOKEN_ENDFOREACH,
		token.TOKEN_ENDTRY, token.TOKEN_CATCH, token.TOKEN_FINALLY,
		token.TOKEN_ENDPARALLEL,
		token.TOKEN_ENDFUNCTION,
		token.TOKEN_ENDLIBRARY,
		token.TOKEN_EOF:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Block parsing
// ---------------------------------------------------------------------------

func (p *Parser) parseBlock(endTokens ...token.TokenType) []ast.Statement {
	var stmts []ast.Statement
	p.skipNewlines()
	for !p.atEnd() {
		for _, et := range endTokens {
			if p.peekType() == et {
				return stmts
			}
		}
		// Also check generic block-end tokens so we don't over-consume
		if isBlockEnd(p.peekType()) {
			// Only stop if this is an actual end token for our enclosing block
			found := false
			for _, et := range endTokens {
				if p.peekType() == et {
					found = true
					break
				}
			}
			if !found {
				// This might be an end token for an outer block; stop here
				return stmts
			}
		}
		stmt := p.parseStatement()
		if stmt != nil {
			stmts = append(stmts, stmt)
		}
		p.skipNewlines()
	}
	return stmts
}

// ---------------------------------------------------------------------------
// Statement dispatch
// ---------------------------------------------------------------------------

func (p *Parser) parseStatement() ast.Statement {
	switch p.peekType() {
	case token.TOKEN_SET:
		return p.parseSetStmt()
	case token.TOKEN_CONST:
		return p.parseConstStmt()
	case token.TOKEN_GLOBAL:
		return p.parseGlobalStmt()
	case token.TOKEN_DELETE:
		return p.parseDeleteStmt()
	case token.TOKEN_APPEND:
		return p.parseAppendStmt()
	case token.TOKEN_EXTEND:
		return p.parseExtendStmt()
	case token.TOKEN_IF:
		return p.parseIfStmt()
	case token.TOKEN_LOOP:
		return p.parseLoopStmt()
	case token.TOKEN_WHILE:
		return p.parseWhileStmt()
	case token.TOKEN_FOREACH:
		return p.parseForEachStmt()
	case token.TOKEN_BREAK:
		return p.parseBreakStmt()
	case token.TOKEN_CONTINUE:
		return p.parseContinueStmt()
	case token.TOKEN_TRY:
		return p.parseTryStmt()
	case token.TOKEN_PARALLEL:
		return p.parseParallelStmt()
	case token.TOKEN_CONNECT:
		return p.parseConnectStmt()
	case token.TOKEN_DISCONNECT:
		return p.parseDisconnectStmt()
	case token.TOKEN_SEND:
		return p.parseSendStmt()
	case token.TOKEN_QUERY:
		return p.parseQueryStmt()
	case token.TOKEN_RELAY:
		return p.parseRelayStmt()
	case token.TOKEN_FUNCTION:
		return p.parseFunctionDef()
	case token.TOKEN_CALL:
		return p.parseCallStmt()
	case token.TOKEN_RETURN:
		return p.parseReturnStmt()
	case token.TOKEN_IMPORT:
		return p.parseImportStmt()
	case token.TOKEN_LIBRARY:
		return p.parseLibraryDef()
	case token.TOKEN_PASS:
		return p.parsePassStmt()
	case token.TOKEN_FAIL:
		return p.parseFailStmt()
	case token.TOKEN_SKIP:
		return p.parseSkipStmt()
	case token.TOKEN_ASSERT:
		return p.parseAssertStmt()
	case token.TOKEN_LOG:
		return p.parseLogStmt()
	case token.TOKEN_DELAY:
		return p.parseDelayStmt()
	case token.TOKEN_RESERVE:
		return p.parseReserveStmt()
	case token.TOKEN_TEST:
		return p.parseTestDef()
	case token.TOKEN_SUITE:
		return p.parseSuiteDef()
	default:
		p.addError(p.peek().Pos, fmt.Sprintf("unexpected token %s", p.peekType()))
		p.synchronize()
		return nil
	}
}

// ---------------------------------------------------------------------------
// Variable statements
// ---------------------------------------------------------------------------

func (p *Parser) parseSetStmt() *ast.SetStmt {
	tok := p.advance() // consume SET
	nameTok := p.expect(token.TOKEN_IDENT)

	node := &ast.SetStmt{
		Name:     nameTok.Literal,
		Position: tok.Pos,
	}

	// Check for indexed assignment: SET arr[i] = value
	// Only treat '[' as index if it is immediately adjacent to the identifier
	// (no whitespace gap), to distinguish from SET arr [1, 2, 3] (array value).
	if p.peekType() == token.TOKEN_LBRACKET {
		bracketPos := p.peek().Pos
		nameEnd := nameTok.Pos.Column + len(nameTok.Literal)
		if bracketPos.Line == nameTok.Pos.Line && bracketPos.Column == nameEnd {
			p.advance() // consume [
			node.Index = p.parseExpression()
			p.expect(token.TOKEN_RBRACKET)
		}
	}

	// Optional '='
	p.match(token.TOKEN_ASSIGN)

	// Check for CALL as value
	if p.peekType() == token.TOKEN_CALL {
		node.Value = p.parseCallExpr()
	} else {
		node.Value = p.parseExpression()
	}

	return node
}

func (p *Parser) parseConstStmt() *ast.ConstStmt {
	tok := p.advance() // consume CONST
	nameTok := p.expect(token.TOKEN_IDENT)

	// Optional '='
	p.match(token.TOKEN_ASSIGN)

	return &ast.ConstStmt{
		Name:     nameTok.Literal,
		Value:    p.parseExpression(),
		Position: tok.Pos,
	}
}

func (p *Parser) parseGlobalStmt() *ast.GlobalStmt {
	tok := p.advance() // consume GLOBAL
	nameTok := p.expect(token.TOKEN_IDENT)

	node := &ast.GlobalStmt{
		Name:     nameTok.Literal,
		Position: tok.Pos,
	}

	// Value is optional
	if !p.atEnd() && p.peekType() != token.TOKEN_NEWLINE && !isBlockEnd(p.peekType()) {
		// Optional '='
		p.match(token.TOKEN_ASSIGN)
		node.Value = p.parseExpression()
	}

	return node
}

func (p *Parser) parseDeleteStmt() *ast.DeleteStmt {
	tok := p.advance() // consume DELETE
	nameTok := p.expect(token.TOKEN_IDENT)
	return &ast.DeleteStmt{
		Name:     nameTok.Literal,
		Position: tok.Pos,
	}
}

func (p *Parser) parseAppendStmt() *ast.AppendStmt {
	tok := p.advance() // consume APPEND
	nameTok := p.expect(token.TOKEN_IDENT)
	value := p.parseExpression()
	return &ast.AppendStmt{
		Name:     nameTok.Literal,
		Value:    value,
		Position: tok.Pos,
	}
}

func (p *Parser) parseExtendStmt() *ast.ExtendStmt {
	tok := p.advance() // consume EXTEND
	nameTok := p.expect(token.TOKEN_IDENT)
	value := p.parseExpression()
	return &ast.ExtendStmt{
		Name:     nameTok.Literal,
		Value:    value,
		Position: tok.Pos,
	}
}

// ---------------------------------------------------------------------------
// Control flow
// ---------------------------------------------------------------------------

func (p *Parser) parseIfStmt() *ast.IfStmt {
	tok := p.advance() // consume IF
	cond := p.parseExpression()

	body := p.parseBlock(token.TOKEN_ELSEIF, token.TOKEN_ELSE, token.TOKEN_ENDIF)

	var elseIfs []ast.ElseIfClause
	for p.peekType() == token.TOKEN_ELSEIF {
		eiTok := p.advance() // consume ELSEIF
		eiCond := p.parseExpression()
		eiBody := p.parseBlock(token.TOKEN_ELSEIF, token.TOKEN_ELSE, token.TOKEN_ENDIF)
		elseIfs = append(elseIfs, ast.ElseIfClause{
			Condition: eiCond,
			Body:      eiBody,
			Position:  eiTok.Pos,
		})
	}

	var elseBody []ast.Statement
	if p.peekType() == token.TOKEN_ELSE {
		p.advance() // consume ELSE
		elseBody = p.parseBlock(token.TOKEN_ENDIF)
	}

	p.expect(token.TOKEN_ENDIF)

	return &ast.IfStmt{
		Condition: cond,
		Body:      body,
		ElseIfs:   elseIfs,
		ElseBody:  elseBody,
		Position:  tok.Pos,
	}
}

func (p *Parser) parseLoopStmt() *ast.LoopStmt {
	tok := p.advance() // consume LOOP
	count := p.parseExpression()
	p.expect(token.TOKEN_TIMES)

	var iterVar string
	if p.peekType() == token.TOKEN_AS {
		p.advance() // consume AS
		iterTok := p.expect(token.TOKEN_IDENT)
		iterVar = iterTok.Literal
	}

	body := p.parseBlock(token.TOKEN_ENDLOOP)
	p.expect(token.TOKEN_ENDLOOP)

	return &ast.LoopStmt{
		Count:    count,
		IterVar:  iterVar,
		Body:     body,
		Position: tok.Pos,
	}
}

func (p *Parser) parseWhileStmt() *ast.WhileStmt {
	tok := p.advance() // consume WHILE
	cond := p.parseExpression()

	body := p.parseBlock(token.TOKEN_ENDWHILE)
	p.expect(token.TOKEN_ENDWHILE)

	return &ast.WhileStmt{
		Condition: cond,
		Body:      body,
		Position:  tok.Pos,
	}
}

func (p *Parser) parseForEachStmt() *ast.ForEachStmt {
	tok := p.advance() // consume FOREACH
	itemTok := p.expect(token.TOKEN_IDENT)
	p.expect(token.TOKEN_IN)
	collection := p.parseExpression()

	var indexVar string
	if p.peekType() == token.TOKEN_AS {
		p.advance() // consume AS
		idxTok := p.expect(token.TOKEN_IDENT)
		indexVar = idxTok.Literal
	}

	body := p.parseBlock(token.TOKEN_ENDFOREACH)
	p.expect(token.TOKEN_ENDFOREACH)

	return &ast.ForEachStmt{
		ItemVar:    itemTok.Literal,
		Collection: collection,
		IndexVar:   indexVar,
		Body:       body,
		Position:   tok.Pos,
	}
}

func (p *Parser) parseBreakStmt() *ast.BreakStmt {
	tok := p.advance() // consume BREAK
	return &ast.BreakStmt{Position: tok.Pos}
}

func (p *Parser) parseContinueStmt() *ast.ContinueStmt {
	tok := p.advance() // consume CONTINUE
	return &ast.ContinueStmt{Position: tok.Pos}
}

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

func (p *Parser) parseTryStmt() *ast.TryStmt {
	tok := p.advance() // consume TRY

	body := p.parseBlock(token.TOKEN_CATCH, token.TOKEN_FINALLY, token.TOKEN_ENDTRY)

	var catchVar string
	var catchBody []ast.Statement
	if p.peekType() == token.TOKEN_CATCH {
		p.advance() // consume CATCH
		varTok := p.expect(token.TOKEN_IDENT)
		catchVar = varTok.Literal
		catchBody = p.parseBlock(token.TOKEN_FINALLY, token.TOKEN_ENDTRY)
	}

	var finallyBody []ast.Statement
	if p.peekType() == token.TOKEN_FINALLY {
		p.advance() // consume FINALLY
		finallyBody = p.parseBlock(token.TOKEN_ENDTRY)
	}

	p.expect(token.TOKEN_ENDTRY)

	return &ast.TryStmt{
		Body:        body,
		CatchVar:    catchVar,
		CatchBody:   catchBody,
		FinallyBody: finallyBody,
		Position:    tok.Pos,
	}
}

// ---------------------------------------------------------------------------
// Parallel
// ---------------------------------------------------------------------------

func (p *Parser) parseParallelStmt() *ast.ParallelStmt {
	tok := p.advance() // consume PARALLEL

	var timeout ast.Expression
	if p.peekType() == token.TOKEN_TIMEOUT {
		p.advance() // consume TIMEOUT
		timeout = p.parseExpression()
	}

	body := p.parseBlock(token.TOKEN_ENDPARALLEL)
	p.expect(token.TOKEN_ENDPARALLEL)

	return &ast.ParallelStmt{
		Timeout:  timeout,
		Body:     body,
		Position: tok.Pos,
	}
}

// ---------------------------------------------------------------------------
// Device communication
// ---------------------------------------------------------------------------

func (p *Parser) parseConnectStmt() *ast.ConnectStmt {
	tok := p.advance() // consume CONNECT
	deviceID := p.expectDeviceID()

	// Protocol: TCP or SERIAL
	var protocol string
	switch p.peekType() {
	case token.TOKEN_TCP:
		protocol = p.advance().Literal
	case token.TOKEN_SERIAL:
		protocol = p.advance().Literal
	default:
		p.addError(p.peek().Pos, "expected TCP or SERIAL")
		protocol = "TCP" // default for recovery
	}

	address := p.parseExpression()

	// Optional extra arguments (e.g. baud rate for SERIAL)
	var options []ast.Expression
	for !p.atEnd() && p.peekType() != token.TOKEN_NEWLINE && !isBlockEnd(p.peekType()) {
		options = append(options, p.parseExpression())
	}

	return &ast.ConnectStmt{
		DeviceID: deviceID,
		Protocol: strings.ToUpper(protocol),
		Address:  address,
		Options:  options,
		Position: tok.Pos,
	}
}

func (p *Parser) parseDisconnectStmt() *ast.DisconnectStmt {
	tok := p.advance() // consume DISCONNECT

	if p.peekType() == token.TOKEN_ALL {
		p.advance() // consume ALL
		return &ast.DisconnectStmt{
			All:      true,
			Position: tok.Pos,
		}
	}

	deviceID := p.expectDeviceID()
	return &ast.DisconnectStmt{
		DeviceID: deviceID,
		Position: tok.Pos,
	}
}

func (p *Parser) parseSendStmt() *ast.SendStmt {
	tok := p.advance() // consume SEND
	deviceID := p.expectDeviceID()
	command := p.parseExpression()

	return &ast.SendStmt{
		DeviceID: deviceID,
		Command:  command,
		Position: tok.Pos,
	}
}

func (p *Parser) parseQueryStmt() *ast.QueryStmt {
	tok := p.advance() // consume QUERY
	deviceID := p.expectDeviceID()
	command := p.parseExpression()
	resultTok := p.expect(token.TOKEN_IDENT)

	node := &ast.QueryStmt{
		DeviceID:  deviceID,
		Command:   command,
		ResultVar: resultTok.Literal,
		Position:  tok.Pos,
	}

	// Optional TIMEOUT
	if p.peekType() == token.TOKEN_TIMEOUT {
		p.advance() // consume TIMEOUT
		node.Timeout = p.parseExpression()
	}

	return node
}

func (p *Parser) parseRelayStmt() *ast.RelayStmt {
	tok := p.advance() // consume RELAY
	deviceID := p.expectDeviceID()

	// Action: SET, GET, or TOGGLE
	var action string
	switch p.peekType() {
	case token.TOKEN_SET:
		action = "SET"
		p.advance()
	case token.TOKEN_GET:
		action = "GET"
		p.advance()
	case token.TOKEN_TOGGLE:
		action = "TOGGLE"
		p.advance()
	default:
		p.addError(p.peek().Pos, "expected SET, GET, or TOGGLE")
		action = "SET" // recovery
	}

	channel := p.parseExpression()

	node := &ast.RelayStmt{
		DeviceID: deviceID,
		Action:   action,
		Channel:  channel,
		Position: tok.Pos,
	}

	// For SET action, expect a state (ON/OFF or expression)
	if action == "SET" {
		if p.peekType() == token.TOKEN_ON {
			node.State = &ast.Identifier{Name: p.advance().Literal, Position: p.tokens[p.pos-1].Pos}
		} else if p.peekType() == token.TOKEN_OFF {
			node.State = &ast.Identifier{Name: p.advance().Literal, Position: p.tokens[p.pos-1].Pos}
		} else if !p.atEnd() && p.peekType() != token.TOKEN_NEWLINE && !isBlockEnd(p.peekType()) {
			node.State = p.parseExpression()
		}
	}

	// For GET action, optional result var
	if action == "GET" {
		if p.peekType() == token.TOKEN_IDENT {
			node.ResultVar = p.advance().Literal
		}
	}

	return node
}

// ---------------------------------------------------------------------------
// Functions
// ---------------------------------------------------------------------------

func (p *Parser) parseFunctionDef() *ast.FunctionDef {
	tok := p.advance() // consume FUNCTION
	nameTok := p.expect(token.TOKEN_IDENT)

	// Parse parameter list: (param1, param2, ...)
	p.expect(token.TOKEN_LPAREN)
	var params []string
	if p.peekType() != token.TOKEN_RPAREN {
		paramTok := p.expect(token.TOKEN_IDENT)
		params = append(params, paramTok.Literal)
		for p.peekType() == token.TOKEN_COMMA {
			p.advance() // consume comma
			paramTok = p.expect(token.TOKEN_IDENT)
			params = append(params, paramTok.Literal)
		}
	}
	p.expect(token.TOKEN_RPAREN)

	body := p.parseBlock(token.TOKEN_ENDFUNCTION)
	p.expect(token.TOKEN_ENDFUNCTION)

	return &ast.FunctionDef{
		Name:     nameTok.Literal,
		Params:   params,
		Body:     body,
		Position: tok.Pos,
	}
}

// parseCallStmt parses a standalone CALL statement (not used as a value).
func (p *Parser) parseCallStmt() ast.Statement {
	callExpr := p.parseCallExpr()
	// Wrap the CallExpr into a SetStmt with empty name, or return a
	// dedicated statement. Since the spec says CALL can be standalone,
	// we wrap it in a SetStmt with no name to indicate a call-as-statement.
	// Actually, let's return it as a SetStmt where the value is the call expr.
	// A simpler approach: return a SetStmt with empty name and Value = callExpr.
	return &ast.SetStmt{
		Name:     "",
		Value:    callExpr,
		Position: callExpr.Pos(),
	}
}

// parseCallExpr parses CALL funcName(args...) as an expression.
func (p *Parser) parseCallExpr() *ast.CallExpr {
	tok := p.advance() // consume CALL
	nameTok := p.expect(token.TOKEN_IDENT)

	p.expect(token.TOKEN_LPAREN)
	var args []ast.Expression
	if p.peekType() != token.TOKEN_RPAREN {
		args = append(args, p.parseExpression())
		for p.peekType() == token.TOKEN_COMMA {
			p.advance() // consume comma
			args = append(args, p.parseExpression())
		}
	}
	p.expect(token.TOKEN_RPAREN)

	return &ast.CallExpr{
		Name:     nameTok.Literal,
		Args:     args,
		Position: tok.Pos,
	}
}

func (p *Parser) parseReturnStmt() *ast.ReturnStmt {
	tok := p.advance() // consume RETURN

	node := &ast.ReturnStmt{Position: tok.Pos}

	// Value is optional; if the next token is a newline or block-end, no value
	if !p.atEnd() && p.peekType() != token.TOKEN_NEWLINE && !isBlockEnd(p.peekType()) {
		node.Value = p.parseExpression()
	}

	return node
}

// ---------------------------------------------------------------------------
// Libraries / imports
// ---------------------------------------------------------------------------

func (p *Parser) parseImportStmt() *ast.ImportStmt {
	tok := p.advance() // consume IMPORT
	path := p.parseExpression()
	return &ast.ImportStmt{
		Path:     path,
		Position: tok.Pos,
	}
}

func (p *Parser) parseLibraryDef() *ast.LibraryDef {
	tok := p.advance() // consume LIBRARY
	name := p.parseExpression()

	body := p.parseBlock(token.TOKEN_ENDLIBRARY)
	p.expect(token.TOKEN_ENDLIBRARY)

	return &ast.LibraryDef{
		Name:     name,
		Body:     body,
		Position: tok.Pos,
	}
}

// ---------------------------------------------------------------------------
// Test results / utilities
// ---------------------------------------------------------------------------

func (p *Parser) parsePassStmt() *ast.PassStmt {
	tok := p.advance() // consume PASS
	msg := p.parseExpression()
	return &ast.PassStmt{Message: msg, Position: tok.Pos}
}

func (p *Parser) parseFailStmt() *ast.FailStmt {
	tok := p.advance() // consume FAIL
	msg := p.parseExpression()
	return &ast.FailStmt{Message: msg, Position: tok.Pos}
}

func (p *Parser) parseSkipStmt() *ast.SkipStmt {
	tok := p.advance() // consume SKIP
	msg := p.parseExpression()
	return &ast.SkipStmt{Message: msg, Position: tok.Pos}
}

func (p *Parser) parseAssertStmt() *ast.AssertStmt {
	tok := p.advance() // consume ASSERT
	cond := p.parseExpression()

	// The message follows the condition; it is a string expression
	var msg ast.Expression
	if p.peekType() == token.TOKEN_STRING {
		msg = p.parseExpression()
	}

	return &ast.AssertStmt{
		Condition: cond,
		Message:   msg,
		Position:  tok.Pos,
	}
}

func (p *Parser) parseLogStmt() *ast.LogStmt {
	tok := p.advance() // consume LOG

	// Level is the next identifier (INFO, WARN, ERROR, DEBUG, etc.)
	levelTok := p.expect(token.TOKEN_IDENT)
	msg := p.parseExpression()

	return &ast.LogStmt{
		Level:    strings.ToUpper(levelTok.Literal),
		Message:  msg,
		Position: tok.Pos,
	}
}

func (p *Parser) parseDelayStmt() *ast.DelayStmt {
	tok := p.advance() // consume DELAY
	dur := p.parseExpression()
	return &ast.DelayStmt{Duration: dur, Position: tok.Pos}
}

func (p *Parser) parseReserveStmt() *ast.ReserveStmt {
	tok := p.advance() // consume RESERVE
	nameTok := p.expect(token.TOKEN_IDENT)
	size := p.parseExpression()
	return &ast.ReserveStmt{
		Name:     nameTok.Literal,
		Size:     size,
		Position: tok.Pos,
	}
}

// ---------------------------------------------------------------------------
// Test / Suite definitions
// ---------------------------------------------------------------------------

func (p *Parser) parseTestDef() *ast.TestDef {
	tok := p.advance() // consume TEST
	name := p.parseExpression()

	body := p.parseBlock(token.TOKEN_ENDTEST)
	p.expect(token.TOKEN_ENDTEST)

	return &ast.TestDef{
		Name:     name,
		Body:     body,
		Position: tok.Pos,
	}
}

func (p *Parser) parseSuiteDef() *ast.SuiteDef {
	tok := p.advance() // consume SUITE
	name := p.parseExpression()

	node := &ast.SuiteDef{
		Name:     name,
		Position: tok.Pos,
	}

	p.skipNewlines()

	// Parse suite contents: SETUP, TEARDOWN, TEST blocks, and other statements
	for !p.atEnd() && p.peekType() != token.TOKEN_ENDSUITE {
		switch p.peekType() {
		case token.TOKEN_SETUP:
			setupTok := p.advance() // consume SETUP
			setupBody := p.parseBlock(token.TOKEN_ENDSETUP)
			p.expect(token.TOKEN_ENDSETUP)
			node.Setup = &ast.SetupBlock{
				Body:     setupBody,
				Position: setupTok.Pos,
			}
		case token.TOKEN_TEARDOWN:
			tearTok := p.advance() // consume TEARDOWN
			tearBody := p.parseBlock(token.TOKEN_ENDTEARDOWN)
			p.expect(token.TOKEN_ENDTEARDOWN)
			node.Teardown = &ast.TeardownBlock{
				Body:     tearBody,
				Position: tearTok.Pos,
			}
		case token.TOKEN_TEST:
			testDef := p.parseTestDef()
			node.Tests = append(node.Tests, testDef)
		default:
			stmt := p.parseStatement()
			if stmt != nil {
				node.Body = append(node.Body, stmt)
			}
		}
		p.skipNewlines()
	}

	p.expect(token.TOKEN_ENDSUITE)

	return node
}

// ---------------------------------------------------------------------------
// Expression parsing (precedence climbing)
// ---------------------------------------------------------------------------

func (p *Parser) parseExpression() ast.Expression {
	return p.parseOr()
}

func (p *Parser) parseOr() ast.Expression {
	left := p.parseAnd()

	for p.peekType() == token.TOKEN_OR {
		opTok := p.advance()
		right := p.parseAnd()
		left = &ast.BinaryExpr{
			Left:     left,
			Op:       opTok.Type,
			Right:    right,
			Position: opTok.Pos,
		}
	}

	return left
}

func (p *Parser) parseAnd() ast.Expression {
	left := p.parseEquality()

	for p.peekType() == token.TOKEN_AND {
		opTok := p.advance()
		right := p.parseEquality()
		left = &ast.BinaryExpr{
			Left:     left,
			Op:       opTok.Type,
			Right:    right,
			Position: opTok.Pos,
		}
	}

	return left
}

func (p *Parser) parseEquality() ast.Expression {
	left := p.parseComparison()

	for p.peekType() == token.TOKEN_EQ || p.peekType() == token.TOKEN_NEQ {
		opTok := p.advance()
		right := p.parseComparison()
		left = &ast.BinaryExpr{
			Left:     left,
			Op:       opTok.Type,
			Right:    right,
			Position: opTok.Pos,
		}
	}

	return left
}

func (p *Parser) parseComparison() ast.Expression {
	left := p.parseAdditive()

	for p.peekType() == token.TOKEN_GT || p.peekType() == token.TOKEN_LT ||
		p.peekType() == token.TOKEN_GTE || p.peekType() == token.TOKEN_LTE {
		opTok := p.advance()
		right := p.parseAdditive()
		left = &ast.BinaryExpr{
			Left:     left,
			Op:       opTok.Type,
			Right:    right,
			Position: opTok.Pos,
		}
	}

	return left
}

func (p *Parser) parseAdditive() ast.Expression {
	left := p.parseMultiplicative()

	for p.peekType() == token.TOKEN_PLUS || p.peekType() == token.TOKEN_MINUS {
		opTok := p.advance()
		right := p.parseMultiplicative()
		left = &ast.BinaryExpr{
			Left:     left,
			Op:       opTok.Type,
			Right:    right,
			Position: opTok.Pos,
		}
	}

	return left
}

func (p *Parser) parseMultiplicative() ast.Expression {
	left := p.parseUnary()

	for p.peekType() == token.TOKEN_STAR || p.peekType() == token.TOKEN_SLASH ||
		p.peekType() == token.TOKEN_PERCENT {
		opTok := p.advance()
		right := p.parseUnary()
		left = &ast.BinaryExpr{
			Left:     left,
			Op:       opTok.Type,
			Right:    right,
			Position: opTok.Pos,
		}
	}

	return left
}

func (p *Parser) parseUnary() ast.Expression {
	if p.peekType() == token.TOKEN_NOT || p.peekType() == token.TOKEN_MINUS {
		opTok := p.advance()
		operand := p.parseUnary()
		return &ast.UnaryExpr{
			Op:       opTok.Type,
			Operand:  operand,
			Position: opTok.Pos,
		}
	}
	return p.parsePostfix(p.parsePrimary())
}

func (p *Parser) parsePostfix(expr ast.Expression) ast.Expression {
	for {
		switch p.peekType() {
		case token.TOKEN_LBRACKET:
			// Index access: expr[index]
			p.advance() // consume [
			index := p.parseExpression()
			rBracket := p.expect(token.TOKEN_RBRACKET)
			expr = &ast.IndexExpr{
				Object:   expr,
				Index:    index,
				Position: rBracket.Pos,
			}
		case token.TOKEN_DOT:
			// Field access: expr.field
			p.advance() // consume .
			fieldTok := p.expect(token.TOKEN_IDENT)
			expr = &ast.IndexExpr{
				Object: expr,
				Index: &ast.StringLit{
					Value:    fieldTok.Literal,
					Position: fieldTok.Pos,
				},
				Position: fieldTok.Pos,
			}
		default:
			return expr
		}
	}
}

func (p *Parser) parsePrimary() ast.Expression {
	tok := p.peek()

	switch tok.Type {
	case token.TOKEN_INT:
		p.advance()
		return &ast.NumberLit{
			Value:    tok.Literal,
			IsFloat:  false,
			Position: tok.Pos,
		}

	case token.TOKEN_FLOAT:
		p.advance()
		return &ast.NumberLit{
			Value:    tok.Literal,
			IsFloat:  true,
			Position: tok.Pos,
		}

	case token.TOKEN_STRING:
		p.advance()
		return &ast.StringLit{
			Value:    tok.Literal,
			Position: tok.Pos,
		}

	case token.TOKEN_TRUE:
		p.advance()
		return &ast.BoolLit{
			Value:    true,
			Position: tok.Pos,
		}

	case token.TOKEN_FALSE:
		p.advance()
		return &ast.BoolLit{
			Value:    false,
			Position: tok.Pos,
		}

	case token.TOKEN_NULL:
		p.advance()
		return &ast.NullLit{Position: tok.Pos}

	case token.TOKEN_IDENT:
		// Check if followed by '(' for builtin call
		if p.peekAt(1).Type == token.TOKEN_LPAREN {
			return p.parseBuiltinCall()
		}
		p.advance()
		return &ast.Identifier{
			Name:     tok.Literal,
			Position: tok.Pos,
		}

	case token.TOKEN_LPAREN:
		// Grouped expression
		p.advance() // consume (
		expr := p.parseExpression()
		p.expect(token.TOKEN_RPAREN)
		return expr

	case token.TOKEN_LBRACKET:
		return p.parseArrayLit()

	case token.TOKEN_LBRACE:
		return p.parseDictLit()

	case token.TOKEN_CALL:
		return p.parseCallExpr()

	default:
		p.addError(tok.Pos, fmt.Sprintf("unexpected token %s in expression", tok.Type))
		p.advance() // skip the bad token
		return &ast.NullLit{Position: tok.Pos}
	}
}

func (p *Parser) parseBuiltinCall() *ast.BuiltinCallExpr {
	nameTok := p.advance() // consume the identifier
	p.advance()            // consume (

	var args []ast.Expression
	if p.peekType() != token.TOKEN_RPAREN {
		args = append(args, p.parseExpression())
		for p.peekType() == token.TOKEN_COMMA {
			p.advance() // consume comma
			args = append(args, p.parseExpression())
		}
	}
	p.expect(token.TOKEN_RPAREN)

	return &ast.BuiltinCallExpr{
		Name:     nameTok.Literal,
		Args:     args,
		Position: nameTok.Pos,
	}
}

func (p *Parser) parseArrayLit() *ast.ArrayLit {
	tok := p.advance() // consume [

	var elems []ast.Expression
	if p.peekType() != token.TOKEN_RBRACKET {
		elems = append(elems, p.parseExpression())
		for p.peekType() == token.TOKEN_COMMA {
			p.advance() // consume comma
			elems = append(elems, p.parseExpression())
		}
	}
	p.expect(token.TOKEN_RBRACKET)

	return &ast.ArrayLit{
		Elements: elems,
		Position: tok.Pos,
	}
}

func (p *Parser) parseDictLit() *ast.DictLit {
	tok := p.advance() // consume {

	var keys, values []ast.Expression
	if p.peekType() != token.TOKEN_RBRACE {
		key := p.parseExpression()
		p.expect(token.TOKEN_COLON)
		val := p.parseExpression()
		keys = append(keys, key)
		values = append(values, val)

		for p.peekType() == token.TOKEN_COMMA {
			p.advance() // consume comma
			key = p.parseExpression()
			p.expect(token.TOKEN_COLON)
			val = p.parseExpression()
			keys = append(keys, key)
			values = append(values, val)
		}
	}
	p.expect(token.TOKEN_RBRACE)

	return &ast.DictLit{
		Keys:     keys,
		Values:   values,
		Position: tok.Pos,
	}
}
