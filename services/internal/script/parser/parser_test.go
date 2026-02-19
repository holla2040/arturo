package parser

import (
	"testing"

	"github.com/holla2040/arturo/internal/script/ast"
	"github.com/holla2040/arturo/internal/script/lexer"
	"github.com/holla2040/arturo/internal/script/token"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseSource lexes + parses source, failing the test on any error.
func parseSource(t *testing.T, source string) *ast.Program {
	t.Helper()
	tokens, lexErrors := lexer.New(source).Tokenize()
	if len(lexErrors) > 0 {
		t.Fatalf("lex errors: %v", lexErrors)
	}
	prog, parseErrors := New(tokens).Parse()
	if len(parseErrors) > 0 {
		t.Fatalf("parse errors: %v", parseErrors)
	}
	return prog
}

// parseSourceWithErrors lexes + parses source, returning the program and any
// parse errors (does not fail on parse errors).
func parseSourceWithErrors(t *testing.T, source string) (*ast.Program, []ParseError) {
	t.Helper()
	tokens, lexErrors := lexer.New(source).Tokenize()
	if len(lexErrors) > 0 {
		t.Fatalf("lex errors: %v", lexErrors)
	}
	return New(tokens).Parse()
}

// requireStmtCount asserts the number of top-level statements.
func requireStmtCount(t *testing.T, prog *ast.Program, n int) {
	t.Helper()
	if len(prog.Statements) != n {
		t.Fatalf("expected %d statements, got %d", n, len(prog.Statements))
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestEmptyProgram(t *testing.T) {
	prog := parseSource(t, "")
	requireStmtCount(t, prog, 0)
}

func TestSetStatement(t *testing.T) {
	prog := parseSource(t, "SET x 5")
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.SetStmt)
	if !ok {
		t.Fatalf("expected *ast.SetStmt, got %T", prog.Statements[0])
	}
	if s.Name != "x" {
		t.Errorf("name: got %q, want %q", s.Name, "x")
	}
	num, ok := s.Value.(*ast.NumberLit)
	if !ok {
		t.Fatalf("expected *ast.NumberLit, got %T", s.Value)
	}
	if num.Value != "5" {
		t.Errorf("value: got %q, want %q", num.Value, "5")
	}
}

func TestSetWithEquals(t *testing.T) {
	prog := parseSource(t, "SET x = 5")
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.SetStmt)
	if s.Name != "x" {
		t.Errorf("name: got %q, want %q", s.Name, "x")
	}
	num := s.Value.(*ast.NumberLit)
	if num.Value != "5" {
		t.Errorf("value: got %q, want %q", num.Value, "5")
	}
}

func TestSetWithFloat(t *testing.T) {
	prog := parseSource(t, "SET voltage 3.14")
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.SetStmt)
	if s.Name != "voltage" {
		t.Errorf("name: got %q, want %q", s.Name, "voltage")
	}
	num := s.Value.(*ast.NumberLit)
	if num.Value != "3.14" || !num.IsFloat {
		t.Errorf("value: got %q (float=%v), want %q (float=true)", num.Value, num.IsFloat, "3.14")
	}
}

func TestSetWithString(t *testing.T) {
	prog := parseSource(t, `SET name "hello"`)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.SetStmt)
	str := s.Value.(*ast.StringLit)
	if str.Value != "hello" {
		t.Errorf("value: got %q, want %q", str.Value, "hello")
	}
}

func TestSetWithExpression(t *testing.T) {
	prog := parseSource(t, "SET total a + b * c")
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.SetStmt)
	// Expect: a + (b * c)  due to precedence
	bin, ok := s.Value.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected *ast.BinaryExpr, got %T", s.Value)
	}
	if bin.Op != token.TOKEN_PLUS {
		t.Errorf("outer op: got %s, want +", bin.Op)
	}
	// Left should be Identifier "a"
	left, ok := bin.Left.(*ast.Identifier)
	if !ok || left.Name != "a" {
		t.Errorf("left: got %v, want Identifier(a)", bin.Left)
	}
	// Right should be BinaryExpr b * c
	right, ok := bin.Right.(*ast.BinaryExpr)
	if !ok || right.Op != token.TOKEN_STAR {
		t.Errorf("right: got %v, want BinaryExpr(*)", bin.Right)
	}
}

func TestConstStatement(t *testing.T) {
	prog := parseSource(t, "CONST MAX 100")
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.ConstStmt)
	if !ok {
		t.Fatalf("expected *ast.ConstStmt, got %T", prog.Statements[0])
	}
	if s.Name != "MAX" {
		t.Errorf("name: got %q, want %q", s.Name, "MAX")
	}
	num := s.Value.(*ast.NumberLit)
	if num.Value != "100" {
		t.Errorf("value: got %q, want %q", num.Value, "100")
	}
}

func TestIfEndif(t *testing.T) {
	src := `IF x > 0
    SET y 1
ENDIF`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("expected *ast.IfStmt, got %T", prog.Statements[0])
	}
	if len(s.Body) != 1 {
		t.Fatalf("body: got %d stmts, want 1", len(s.Body))
	}
	if len(s.ElseIfs) != 0 {
		t.Errorf("elseifs: got %d, want 0", len(s.ElseIfs))
	}
	if s.ElseBody != nil {
		t.Errorf("else body should be nil")
	}
}

func TestIfElseEndif(t *testing.T) {
	src := `IF x > 0
    SET y 1
ELSE
    SET y 0
ENDIF`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.IfStmt)
	if len(s.Body) != 1 {
		t.Fatalf("if body: got %d stmts, want 1", len(s.Body))
	}
	if len(s.ElseBody) != 1 {
		t.Fatalf("else body: got %d stmts, want 1", len(s.ElseBody))
	}
}

func TestIfElseIfElseEndif(t *testing.T) {
	src := `IF x > 10
    SET y 3
ELSEIF x > 5
    SET y 2
ELSEIF x > 0
    SET y 1
ELSE
    SET y 0
ENDIF`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.IfStmt)
	if len(s.ElseIfs) != 2 {
		t.Fatalf("elseifs: got %d, want 2", len(s.ElseIfs))
	}
	if len(s.ElseBody) != 1 {
		t.Fatalf("else body: got %d stmts, want 1", len(s.ElseBody))
	}
}

func TestNestedIf(t *testing.T) {
	src := `IF a > 0
    IF b > 0
        SET c 1
    ENDIF
ENDIF`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	outer := prog.Statements[0].(*ast.IfStmt)
	if len(outer.Body) != 1 {
		t.Fatalf("outer body: got %d stmts, want 1", len(outer.Body))
	}
	inner, ok := outer.Body[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("expected nested IfStmt, got %T", outer.Body[0])
	}
	if len(inner.Body) != 1 {
		t.Fatalf("inner body: got %d stmts, want 1", len(inner.Body))
	}
}

func TestLoopTimes(t *testing.T) {
	src := `LOOP 10 TIMES
    SET x 1
ENDLOOP`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.LoopStmt)
	if !ok {
		t.Fatalf("expected *ast.LoopStmt, got %T", prog.Statements[0])
	}
	num := s.Count.(*ast.NumberLit)
	if num.Value != "10" {
		t.Errorf("count: got %q, want %q", num.Value, "10")
	}
	if s.IterVar != "" {
		t.Errorf("itervar: got %q, want empty", s.IterVar)
	}
	if len(s.Body) != 1 {
		t.Fatalf("body: got %d stmts, want 1", len(s.Body))
	}
}

func TestLoopTimesWithAS(t *testing.T) {
	src := `LOOP 10 TIMES AS i
    SET x i
ENDLOOP`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.LoopStmt)
	if s.IterVar != "i" {
		t.Errorf("itervar: got %q, want %q", s.IterVar, "i")
	}
}

func TestWhileLoop(t *testing.T) {
	src := `WHILE x > 0
    SET x x - 1
ENDWHILE`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.WhileStmt)
	if !ok {
		t.Fatalf("expected *ast.WhileStmt, got %T", prog.Statements[0])
	}
	if len(s.Body) != 1 {
		t.Fatalf("body: got %d stmts, want 1", len(s.Body))
	}
}

func TestForEachLoop(t *testing.T) {
	src := `FOREACH item IN items
    LOG INFO item
ENDFOREACH`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.ForEachStmt)
	if !ok {
		t.Fatalf("expected *ast.ForEachStmt, got %T", prog.Statements[0])
	}
	if s.ItemVar != "item" {
		t.Errorf("itemvar: got %q, want %q", s.ItemVar, "item")
	}
	ident := s.Collection.(*ast.Identifier)
	if ident.Name != "items" {
		t.Errorf("collection: got %q, want %q", ident.Name, "items")
	}
	if s.IndexVar != "" {
		t.Errorf("indexvar: got %q, want empty", s.IndexVar)
	}
}

func TestForEachWithIndex(t *testing.T) {
	src := `FOREACH item IN items AS i
    LOG INFO item
ENDFOREACH`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.ForEachStmt)
	if s.IndexVar != "i" {
		t.Errorf("indexvar: got %q, want %q", s.IndexVar, "i")
	}
}

func TestBreakAndContinue(t *testing.T) {
	src := `LOOP 10 TIMES
    IF x > 5
        BREAK
    ENDIF
    CONTINUE
ENDLOOP`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	loop := prog.Statements[0].(*ast.LoopStmt)
	// body should have IF and CONTINUE
	if len(loop.Body) != 2 {
		t.Fatalf("body: got %d stmts, want 2", len(loop.Body))
	}
	ifStmt := loop.Body[0].(*ast.IfStmt)
	_, isBreak := ifStmt.Body[0].(*ast.BreakStmt)
	if !isBreak {
		t.Errorf("expected BreakStmt inside IF, got %T", ifStmt.Body[0])
	}
	_, isContinue := loop.Body[1].(*ast.ContinueStmt)
	if !isContinue {
		t.Errorf("expected ContinueStmt, got %T", loop.Body[1])
	}
}

func TestTryCatch(t *testing.T) {
	src := `TRY
    SEND dmm "*RST"
CATCH err
    LOG ERROR err
ENDTRY`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.TryStmt)
	if !ok {
		t.Fatalf("expected *ast.TryStmt, got %T", prog.Statements[0])
	}
	if len(s.Body) != 1 {
		t.Fatalf("try body: got %d stmts, want 1", len(s.Body))
	}
	if s.CatchVar != "err" {
		t.Errorf("catch var: got %q, want %q", s.CatchVar, "err")
	}
	if len(s.CatchBody) != 1 {
		t.Fatalf("catch body: got %d stmts, want 1", len(s.CatchBody))
	}
	if s.FinallyBody != nil {
		t.Errorf("finally body should be nil")
	}
}

func TestTryCatchFinally(t *testing.T) {
	src := `TRY
    SEND dmm "*RST"
CATCH err
    LOG ERROR err
FINALLY
    DISCONNECT dmm
ENDTRY`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.TryStmt)
	if len(s.Body) != 1 {
		t.Fatalf("try body: got %d stmts, want 1", len(s.Body))
	}
	if len(s.CatchBody) != 1 {
		t.Fatalf("catch body: got %d stmts, want 1", len(s.CatchBody))
	}
	if len(s.FinallyBody) != 1 {
		t.Fatalf("finally body: got %d stmts, want 1", len(s.FinallyBody))
	}
}

func TestConnectTCP(t *testing.T) {
	src := `CONNECT dmm TCP "10.0.0.1:5025"`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.ConnectStmt)
	if !ok {
		t.Fatalf("expected *ast.ConnectStmt, got %T", prog.Statements[0])
	}
	if s.DeviceID != "dmm" {
		t.Errorf("device: got %q, want %q", s.DeviceID, "dmm")
	}
	if s.Protocol != "TCP" {
		t.Errorf("protocol: got %q, want %q", s.Protocol, "TCP")
	}
	addr := s.Address.(*ast.StringLit)
	if addr.Value != "10.0.0.1:5025" {
		t.Errorf("address: got %q, want %q", addr.Value, "10.0.0.1:5025")
	}
	if len(s.Options) != 0 {
		t.Errorf("options: got %d, want 0", len(s.Options))
	}
}

func TestConnectSerial(t *testing.T) {
	src := `CONNECT pump SERIAL "/dev/ttyUSB0" 9600`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.ConnectStmt)
	if s.DeviceID != "pump" {
		t.Errorf("device: got %q, want %q", s.DeviceID, "pump")
	}
	if s.Protocol != "SERIAL" {
		t.Errorf("protocol: got %q, want %q", s.Protocol, "SERIAL")
	}
	if len(s.Options) != 1 {
		t.Fatalf("options: got %d, want 1", len(s.Options))
	}
	baud := s.Options[0].(*ast.NumberLit)
	if baud.Value != "9600" {
		t.Errorf("baud: got %q, want %q", baud.Value, "9600")
	}
}

func TestDisconnect(t *testing.T) {
	prog := parseSource(t, "DISCONNECT dmm")
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.DisconnectStmt)
	if !ok {
		t.Fatalf("expected *ast.DisconnectStmt, got %T", prog.Statements[0])
	}
	if s.DeviceID != "dmm" {
		t.Errorf("device: got %q, want %q", s.DeviceID, "dmm")
	}
	if s.All {
		t.Errorf("all: got true, want false")
	}
}

func TestDisconnectAll(t *testing.T) {
	prog := parseSource(t, "DISCONNECT ALL")
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.DisconnectStmt)
	if !s.All {
		t.Errorf("all: got false, want true")
	}
}

func TestSend(t *testing.T) {
	src := `SEND dmm "*RST"`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.SendStmt)
	if !ok {
		t.Fatalf("expected *ast.SendStmt, got %T", prog.Statements[0])
	}
	if s.DeviceID != "dmm" {
		t.Errorf("device: got %q, want %q", s.DeviceID, "dmm")
	}
	cmd := s.Command.(*ast.StringLit)
	if cmd.Value != "*RST" {
		t.Errorf("command: got %q, want %q", cmd.Value, "*RST")
	}
}

func TestQuery(t *testing.T) {
	src := `QUERY dmm "MEAS:VOLT?" voltage`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.QueryStmt)
	if !ok {
		t.Fatalf("expected *ast.QueryStmt, got %T", prog.Statements[0])
	}
	if s.DeviceID != "dmm" {
		t.Errorf("device: got %q, want %q", s.DeviceID, "dmm")
	}
	cmd := s.Command.(*ast.StringLit)
	if cmd.Value != "MEAS:VOLT?" {
		t.Errorf("command: got %q, want %q", cmd.Value, "MEAS:VOLT?")
	}
	if s.ResultVar != "voltage" {
		t.Errorf("result var: got %q, want %q", s.ResultVar, "voltage")
	}
	if s.Timeout != nil {
		t.Errorf("timeout should be nil")
	}
}

func TestQueryWithTimeout(t *testing.T) {
	src := `QUERY dmm "MEAS:VOLT?" voltage TIMEOUT 5000`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.QueryStmt)
	if s.Timeout == nil {
		t.Fatal("timeout should not be nil")
	}
	num := s.Timeout.(*ast.NumberLit)
	if num.Value != "5000" {
		t.Errorf("timeout: got %q, want %q", num.Value, "5000")
	}
}

func TestRelaySet(t *testing.T) {
	src := "RELAY board SET 1 ON"
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.RelayStmt)
	if !ok {
		t.Fatalf("expected *ast.RelayStmt, got %T", prog.Statements[0])
	}
	if s.DeviceID != "board" {
		t.Errorf("device: got %q, want %q", s.DeviceID, "board")
	}
	if s.Action != "SET" {
		t.Errorf("action: got %q, want %q", s.Action, "SET")
	}
	ch := s.Channel.(*ast.NumberLit)
	if ch.Value != "1" {
		t.Errorf("channel: got %q, want %q", ch.Value, "1")
	}
	if s.State == nil {
		t.Fatal("state should not be nil")
	}
}

func TestFunctionDef(t *testing.T) {
	src := `FUNCTION measure(device, channel)
    QUERY device "MEAS?" result
    RETURN result
ENDFUNCTION`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.FunctionDef)
	if !ok {
		t.Fatalf("expected *ast.FunctionDef, got %T", prog.Statements[0])
	}
	if s.Name != "measure" {
		t.Errorf("name: got %q, want %q", s.Name, "measure")
	}
	if len(s.Params) != 2 {
		t.Fatalf("params: got %d, want 2", len(s.Params))
	}
	if s.Params[0] != "device" || s.Params[1] != "channel" {
		t.Errorf("params: got %v, want [device, channel]", s.Params)
	}
	if len(s.Body) != 2 {
		t.Fatalf("body: got %d stmts, want 2", len(s.Body))
	}
}

func TestCallStatement(t *testing.T) {
	src := "CALL measure(dmm, 101)"
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	// Standalone CALL is wrapped in a SetStmt with empty name
	s, ok := prog.Statements[0].(*ast.SetStmt)
	if !ok {
		t.Fatalf("expected *ast.SetStmt, got %T", prog.Statements[0])
	}
	call, ok := s.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected *ast.CallExpr, got %T", s.Value)
	}
	if call.Name != "measure" {
		t.Errorf("name: got %q, want %q", call.Name, "measure")
	}
	if len(call.Args) != 2 {
		t.Fatalf("args: got %d, want 2", len(call.Args))
	}
}

func TestSetWithCall(t *testing.T) {
	src := "SET result CALL measure(dmm, 101)"
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.SetStmt)
	if s.Name != "result" {
		t.Errorf("name: got %q, want %q", s.Name, "result")
	}
	call, ok := s.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected *ast.CallExpr, got %T", s.Value)
	}
	if call.Name != "measure" {
		t.Errorf("call name: got %q, want %q", call.Name, "measure")
	}
}

func TestReturnStatement(t *testing.T) {
	src := `FUNCTION f()
    RETURN 42
ENDFUNCTION`
	prog := parseSource(t, src)
	fn := prog.Statements[0].(*ast.FunctionDef)
	ret, ok := fn.Body[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("expected *ast.ReturnStmt, got %T", fn.Body[0])
	}
	num := ret.Value.(*ast.NumberLit)
	if num.Value != "42" {
		t.Errorf("return value: got %q, want %q", num.Value, "42")
	}
}

func TestImportStatement(t *testing.T) {
	src := `IMPORT "lib.artlib"`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.ImportStmt)
	if !ok {
		t.Fatalf("expected *ast.ImportStmt, got %T", prog.Statements[0])
	}
	path := s.Path.(*ast.StringLit)
	if path.Value != "lib.artlib" {
		t.Errorf("path: got %q, want %q", path.Value, "lib.artlib")
	}
}

func TestTestBlock(t *testing.T) {
	src := `TEST "My Test"
    SET x 1
ENDTEST`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.TestDef)
	if !ok {
		t.Fatalf("expected *ast.TestDef, got %T", prog.Statements[0])
	}
	name := s.Name.(*ast.StringLit)
	if name.Value != "My Test" {
		t.Errorf("name: got %q, want %q", name.Value, "My Test")
	}
	if len(s.Body) != 1 {
		t.Fatalf("body: got %d stmts, want 1", len(s.Body))
	}
}

func TestSuiteWithSetupTeardown(t *testing.T) {
	src := `SUITE "My Suite"
    SETUP
        CONNECT dmm TCP "10.0.0.1:5025"
    ENDSETUP
    TEARDOWN
        DISCONNECT dmm
    ENDTEARDOWN
    TEST "Test 1"
        SET x 1
    ENDTEST
    TEST "Test 2"
        SET x 2
    ENDTEST
ENDSUITE`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.SuiteDef)
	if !ok {
		t.Fatalf("expected *ast.SuiteDef, got %T", prog.Statements[0])
	}
	name := s.Name.(*ast.StringLit)
	if name.Value != "My Suite" {
		t.Errorf("name: got %q, want %q", name.Value, "My Suite")
	}
	if s.Setup == nil {
		t.Fatal("setup should not be nil")
	}
	if len(s.Setup.Body) != 1 {
		t.Fatalf("setup body: got %d stmts, want 1", len(s.Setup.Body))
	}
	if s.Teardown == nil {
		t.Fatal("teardown should not be nil")
	}
	if len(s.Teardown.Body) != 1 {
		t.Fatalf("teardown body: got %d stmts, want 1", len(s.Teardown.Body))
	}
	if len(s.Tests) != 2 {
		t.Fatalf("tests: got %d, want 2", len(s.Tests))
	}
}

func TestPassFailSkip(t *testing.T) {
	tests := []struct {
		name string
		src  string
		typ  interface{}
	}{
		{"pass", `PASS "OK"`, &ast.PassStmt{}},
		{"fail", `FAIL "bad"`, &ast.FailStmt{}},
		{"skip", `SKIP "not ready"`, &ast.SkipStmt{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prog := parseSource(t, tc.src)
			requireStmtCount(t, prog, 1)
			// Just verify the types
			switch tc.name {
			case "pass":
				s, ok := prog.Statements[0].(*ast.PassStmt)
				if !ok {
					t.Fatalf("expected *ast.PassStmt, got %T", prog.Statements[0])
				}
				msg := s.Message.(*ast.StringLit)
				if msg.Value != "OK" {
					t.Errorf("message: got %q, want %q", msg.Value, "OK")
				}
			case "fail":
				s, ok := prog.Statements[0].(*ast.FailStmt)
				if !ok {
					t.Fatalf("expected *ast.FailStmt, got %T", prog.Statements[0])
				}
				msg := s.Message.(*ast.StringLit)
				if msg.Value != "bad" {
					t.Errorf("message: got %q, want %q", msg.Value, "bad")
				}
			case "skip":
				s, ok := prog.Statements[0].(*ast.SkipStmt)
				if !ok {
					t.Fatalf("expected *ast.SkipStmt, got %T", prog.Statements[0])
				}
				msg := s.Message.(*ast.StringLit)
				if msg.Value != "not ready" {
					t.Errorf("message: got %q, want %q", msg.Value, "not ready")
				}
			}
		})
	}
}

func TestAssertStatement(t *testing.T) {
	src := `ASSERT voltage > 0 "Must be positive"`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.AssertStmt)
	if !ok {
		t.Fatalf("expected *ast.AssertStmt, got %T", prog.Statements[0])
	}
	// Condition should be a binary expression: voltage > 0
	bin := s.Condition.(*ast.BinaryExpr)
	if bin.Op != token.TOKEN_GT {
		t.Errorf("op: got %s, want >", bin.Op)
	}
	msg := s.Message.(*ast.StringLit)
	if msg.Value != "Must be positive" {
		t.Errorf("message: got %q, want %q", msg.Value, "Must be positive")
	}
}

func TestLogStatement(t *testing.T) {
	src := `LOG INFO "message"`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.LogStmt)
	if !ok {
		t.Fatalf("expected *ast.LogStmt, got %T", prog.Statements[0])
	}
	if s.Level != "INFO" {
		t.Errorf("level: got %q, want %q", s.Level, "INFO")
	}
	msg := s.Message.(*ast.StringLit)
	if msg.Value != "message" {
		t.Errorf("message: got %q, want %q", msg.Value, "message")
	}
}

func TestDelayStatement(t *testing.T) {
	prog := parseSource(t, "DELAY 1000")
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.DelayStmt)
	if !ok {
		t.Fatalf("expected *ast.DelayStmt, got %T", prog.Statements[0])
	}
	num := s.Duration.(*ast.NumberLit)
	if num.Value != "1000" {
		t.Errorf("duration: got %q, want %q", num.Value, "1000")
	}
}

func TestParallelBlock(t *testing.T) {
	src := `PARALLEL
    SEND dmm "*RST"
    SEND psu "*RST"
ENDPARALLEL`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s, ok := prog.Statements[0].(*ast.ParallelStmt)
	if !ok {
		t.Fatalf("expected *ast.ParallelStmt, got %T", prog.Statements[0])
	}
	if s.Timeout != nil {
		t.Errorf("timeout should be nil")
	}
	if len(s.Body) != 2 {
		t.Fatalf("body: got %d stmts, want 2", len(s.Body))
	}
}

func TestArrayLiteral(t *testing.T) {
	src := "SET arr [1, 2, 3]"
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.SetStmt)
	arr, ok := s.Value.(*ast.ArrayLit)
	if !ok {
		t.Fatalf("expected *ast.ArrayLit, got %T", s.Value)
	}
	if len(arr.Elements) != 3 {
		t.Fatalf("elements: got %d, want 3", len(arr.Elements))
	}
	for i, expected := range []string{"1", "2", "3"} {
		num := arr.Elements[i].(*ast.NumberLit)
		if num.Value != expected {
			t.Errorf("element[%d]: got %q, want %q", i, num.Value, expected)
		}
	}
}

func TestDictLiteral(t *testing.T) {
	src := `SET d {"key": "value"}`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.SetStmt)
	dict, ok := s.Value.(*ast.DictLit)
	if !ok {
		t.Fatalf("expected *ast.DictLit, got %T", s.Value)
	}
	if len(dict.Keys) != 1 || len(dict.Values) != 1 {
		t.Fatalf("dict: got %d keys, %d values", len(dict.Keys), len(dict.Values))
	}
	key := dict.Keys[0].(*ast.StringLit)
	if key.Value != "key" {
		t.Errorf("key: got %q, want %q", key.Value, "key")
	}
	val := dict.Values[0].(*ast.StringLit)
	if val.Value != "value" {
		t.Errorf("value: got %q, want %q", val.Value, "value")
	}
}

func TestIndexExpression(t *testing.T) {
	src := "SET x arr[0]"
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.SetStmt)
	idx, ok := s.Value.(*ast.IndexExpr)
	if !ok {
		t.Fatalf("expected *ast.IndexExpr, got %T", s.Value)
	}
	obj := idx.Object.(*ast.Identifier)
	if obj.Name != "arr" {
		t.Errorf("object: got %q, want %q", obj.Name, "arr")
	}
	num := idx.Index.(*ast.NumberLit)
	if num.Value != "0" {
		t.Errorf("index: got %q, want %q", num.Value, "0")
	}
}

func TestBuiltinCall(t *testing.T) {
	src := "SET len LENGTH(arr)"
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.SetStmt)
	call, ok := s.Value.(*ast.BuiltinCallExpr)
	if !ok {
		t.Fatalf("expected *ast.BuiltinCallExpr, got %T", s.Value)
	}
	if call.Name != "LENGTH" {
		t.Errorf("name: got %q, want %q", call.Name, "LENGTH")
	}
	if len(call.Args) != 1 {
		t.Fatalf("args: got %d, want 1", len(call.Args))
	}
	arg := call.Args[0].(*ast.Identifier)
	if arg.Name != "arr" {
		t.Errorf("arg: got %q, want %q", arg.Name, "arr")
	}
}

func TestComplexExpressionPrecedence(t *testing.T) {
	// a + b * c >= d && e || !f
	// Expected tree:
	//   ||
	//   /  \
	// &&    !f
	// / \
	// >=  e
	// / \
	// +   d
	// / \
	// a  *
	//   / \
	//   b   c
	src := "SET x a + b * c >= d && e || !f"
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	s := prog.Statements[0].(*ast.SetStmt)

	// Top level should be OR
	or, ok := s.Value.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected *ast.BinaryExpr (OR), got %T", s.Value)
	}
	if or.Op != token.TOKEN_OR {
		t.Errorf("top op: got %s, want ||", or.Op)
	}

	// Left of OR should be AND
	and, ok := or.Left.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected *ast.BinaryExpr (AND), got %T", or.Left)
	}
	if and.Op != token.TOKEN_AND {
		t.Errorf("and op: got %s, want &&", and.Op)
	}

	// Right of OR should be UnaryExpr NOT
	not, ok := or.Right.(*ast.UnaryExpr)
	if !ok {
		t.Fatalf("expected *ast.UnaryExpr (NOT), got %T", or.Right)
	}
	if not.Op != token.TOKEN_NOT {
		t.Errorf("not op: got %s, want !", not.Op)
	}

	// Left of AND should be GTE
	gte, ok := and.Left.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected *ast.BinaryExpr (GTE), got %T", and.Left)
	}
	if gte.Op != token.TOKEN_GTE {
		t.Errorf("gte op: got %s, want >=", gte.Op)
	}

	// Left of GTE should be PLUS
	plus, ok := gte.Left.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected *ast.BinaryExpr (PLUS), got %T", gte.Left)
	}
	if plus.Op != token.TOKEN_PLUS {
		t.Errorf("plus op: got %s, want +", plus.Op)
	}

	// Right of PLUS should be STAR
	star, ok := plus.Right.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected *ast.BinaryExpr (STAR), got %T", plus.Right)
	}
	if star.Op != token.TOKEN_STAR {
		t.Errorf("star op: got %s, want *", star.Op)
	}
}

func TestErrorRecovery(t *testing.T) {
	// Multiple bad statements; parser should report multiple errors
	src := `@ something
SET x 5
@ another`
	tokens, lexErrors := lexer.New(src).Tokenize()
	// Expect lex errors for '@'
	if len(lexErrors) == 0 {
		t.Fatal("expected lex errors")
	}
	_, parseErrors := New(tokens).Parse()
	// Should have at least one parse error (for the ILLEGAL tokens)
	if len(parseErrors) == 0 {
		t.Fatal("expected parse errors")
	}
}

func TestNestedBlocks(t *testing.T) {
	src := `SUITE "Integration"
    SETUP
        CONNECT dmm TCP "10.0.0.1:5025"
    ENDSETUP
    TEST "Voltage Sweep"
        LOOP 5 TIMES AS i
            IF i > 2
                SET voltage i * 1.1
            ENDIF
        ENDLOOP
    ENDTEST
ENDSUITE`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	suite := prog.Statements[0].(*ast.SuiteDef)
	if suite.Setup == nil {
		t.Fatal("setup should not be nil")
	}
	if len(suite.Tests) != 1 {
		t.Fatalf("tests: got %d, want 1", len(suite.Tests))
	}
	test := suite.Tests[0]
	if len(test.Body) != 1 {
		t.Fatalf("test body: got %d stmts, want 1", len(test.Body))
	}
	loop := test.Body[0].(*ast.LoopStmt)
	if loop.IterVar != "i" {
		t.Errorf("itervar: got %q, want %q", loop.IterVar, "i")
	}
	if len(loop.Body) != 1 {
		t.Fatalf("loop body: got %d stmts, want 1", len(loop.Body))
	}
	ifStmt := loop.Body[0].(*ast.IfStmt)
	if len(ifStmt.Body) != 1 {
		t.Fatalf("if body: got %d stmts, want 1", len(ifStmt.Body))
	}
}

func TestDeleteAppendExtendGlobalReserve(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"delete", "DELETE x"},
		{"append", "APPEND arr 42"},
		{"extend", "EXTEND arr [1, 2]"},
		{"global", "GLOBAL counter 0"},
		{"global_no_value", "GLOBAL counter"},
		{"reserve", "RESERVE buf 1024"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prog := parseSource(t, tc.src)
			requireStmtCount(t, prog, 1)
			switch tc.name {
			case "delete":
				s, ok := prog.Statements[0].(*ast.DeleteStmt)
				if !ok {
					t.Fatalf("expected *ast.DeleteStmt, got %T", prog.Statements[0])
				}
				if s.Name != "x" {
					t.Errorf("name: got %q, want %q", s.Name, "x")
				}
			case "append":
				s, ok := prog.Statements[0].(*ast.AppendStmt)
				if !ok {
					t.Fatalf("expected *ast.AppendStmt, got %T", prog.Statements[0])
				}
				if s.Name != "arr" {
					t.Errorf("name: got %q, want %q", s.Name, "arr")
				}
			case "extend":
				s, ok := prog.Statements[0].(*ast.ExtendStmt)
				if !ok {
					t.Fatalf("expected *ast.ExtendStmt, got %T", prog.Statements[0])
				}
				if s.Name != "arr" {
					t.Errorf("name: got %q, want %q", s.Name, "arr")
				}
			case "global":
				s, ok := prog.Statements[0].(*ast.GlobalStmt)
				if !ok {
					t.Fatalf("expected *ast.GlobalStmt, got %T", prog.Statements[0])
				}
				if s.Name != "counter" {
					t.Errorf("name: got %q, want %q", s.Name, "counter")
				}
				if s.Value == nil {
					t.Error("value should not be nil")
				}
			case "global_no_value":
				s, ok := prog.Statements[0].(*ast.GlobalStmt)
				if !ok {
					t.Fatalf("expected *ast.GlobalStmt, got %T", prog.Statements[0])
				}
				if s.Value != nil {
					t.Error("value should be nil")
				}
			case "reserve":
				s, ok := prog.Statements[0].(*ast.ReserveStmt)
				if !ok {
					t.Fatalf("expected *ast.ReserveStmt, got %T", prog.Statements[0])
				}
				if s.Name != "buf" {
					t.Errorf("name: got %q, want %q", s.Name, "buf")
				}
			}
		})
	}
}

func TestBooleanAndNullLiterals(t *testing.T) {
	tests := []struct {
		name string
		src  string
		val  interface{}
	}{
		{"true", "SET x TRUE", true},
		{"false", "SET x FALSE", false},
		{"null", "SET x NULL", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prog := parseSource(t, tc.src)
			s := prog.Statements[0].(*ast.SetStmt)
			switch tc.name {
			case "true":
				b := s.Value.(*ast.BoolLit)
				if !b.Value {
					t.Error("expected true")
				}
			case "false":
				b := s.Value.(*ast.BoolLit)
				if b.Value {
					t.Error("expected false")
				}
			case "null":
				_, ok := s.Value.(*ast.NullLit)
				if !ok {
					t.Fatalf("expected *ast.NullLit, got %T", s.Value)
				}
			}
		})
	}
}

func TestParseErrorType(t *testing.T) {
	e := ParseError{
		Line:     5,
		Column:   10,
		Severity: "error",
		Message:  "unexpected token",
	}
	got := e.Error()
	want := "line 5, column 10: error: unexpected token"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMultipleStatements(t *testing.T) {
	src := `SET a 1
SET b 2
SET c 3`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 3)
	for i, expected := range []string{"a", "b", "c"} {
		s := prog.Statements[i].(*ast.SetStmt)
		if s.Name != expected {
			t.Errorf("stmt[%d] name: got %q, want %q", i, s.Name, expected)
		}
	}
}

func TestDotFieldAccess(t *testing.T) {
	src := "SET x obj.field"
	prog := parseSource(t, src)
	s := prog.Statements[0].(*ast.SetStmt)
	idx, ok := s.Value.(*ast.IndexExpr)
	if !ok {
		t.Fatalf("expected *ast.IndexExpr, got %T", s.Value)
	}
	obj := idx.Object.(*ast.Identifier)
	if obj.Name != "obj" {
		t.Errorf("object: got %q, want %q", obj.Name, "obj")
	}
	field := idx.Index.(*ast.StringLit)
	if field.Value != "field" {
		t.Errorf("field: got %q, want %q", field.Value, "field")
	}
}

func TestGroupedExpression(t *testing.T) {
	src := "SET x (a + b) * c"
	prog := parseSource(t, src)
	s := prog.Statements[0].(*ast.SetStmt)
	// Top level should be *
	bin := s.Value.(*ast.BinaryExpr)
	if bin.Op != token.TOKEN_STAR {
		t.Errorf("op: got %s, want *", bin.Op)
	}
	// Left should be a + b (grouped)
	inner := bin.Left.(*ast.BinaryExpr)
	if inner.Op != token.TOKEN_PLUS {
		t.Errorf("inner op: got %s, want +", inner.Op)
	}
}

func TestUnaryNegation(t *testing.T) {
	src := "SET x -5"
	prog := parseSource(t, src)
	s := prog.Statements[0].(*ast.SetStmt)
	un, ok := s.Value.(*ast.UnaryExpr)
	if !ok {
		t.Fatalf("expected *ast.UnaryExpr, got %T", s.Value)
	}
	if un.Op != token.TOKEN_MINUS {
		t.Errorf("op: got %s, want -", un.Op)
	}
	num := un.Operand.(*ast.NumberLit)
	if num.Value != "5" {
		t.Errorf("operand: got %q, want %q", num.Value, "5")
	}
}

func TestLibraryDef(t *testing.T) {
	src := `LIBRARY "mylib"
    FUNCTION helper()
        RETURN 1
    ENDFUNCTION
ENDLIBRARY`
	prog := parseSource(t, src)
	requireStmtCount(t, prog, 1)
	lib, ok := prog.Statements[0].(*ast.LibraryDef)
	if !ok {
		t.Fatalf("expected *ast.LibraryDef, got %T", prog.Statements[0])
	}
	name := lib.Name.(*ast.StringLit)
	if name.Value != "mylib" {
		t.Errorf("name: got %q, want %q", name.Value, "mylib")
	}
	if len(lib.Body) != 1 {
		t.Fatalf("body: got %d stmts, want 1", len(lib.Body))
	}
	_, isFn := lib.Body[0].(*ast.FunctionDef)
	if !isFn {
		t.Fatalf("expected FunctionDef in library, got %T", lib.Body[0])
	}
}

func TestParallelWithTimeout(t *testing.T) {
	src := `PARALLEL TIMEOUT 5000
    SEND dmm "*RST"
ENDPARALLEL`
	prog := parseSource(t, src)
	s := prog.Statements[0].(*ast.ParallelStmt)
	if s.Timeout == nil {
		t.Fatal("timeout should not be nil")
	}
	num := s.Timeout.(*ast.NumberLit)
	if num.Value != "5000" {
		t.Errorf("timeout: got %q, want %q", num.Value, "5000")
	}
}

func TestSetWithIndex(t *testing.T) {
	src := "SET arr[0] = 42"
	prog := parseSource(t, src)
	s := prog.Statements[0].(*ast.SetStmt)
	if s.Name != "arr" {
		t.Errorf("name: got %q, want %q", s.Name, "arr")
	}
	if s.Index == nil {
		t.Fatal("index should not be nil")
	}
	idx := s.Index.(*ast.NumberLit)
	if idx.Value != "0" {
		t.Errorf("index: got %q, want %q", idx.Value, "0")
	}
	num := s.Value.(*ast.NumberLit)
	if num.Value != "42" {
		t.Errorf("value: got %q, want %q", num.Value, "42")
	}
}

func TestReturnEmpty(t *testing.T) {
	src := `FUNCTION f()
    RETURN
ENDFUNCTION`
	prog := parseSource(t, src)
	fn := prog.Statements[0].(*ast.FunctionDef)
	ret := fn.Body[0].(*ast.ReturnStmt)
	if ret.Value != nil {
		t.Errorf("expected nil return value, got %T", ret.Value)
	}
}

func TestChainedIndexAndDot(t *testing.T) {
	src := "SET x arr[0].field"
	prog := parseSource(t, src)
	s := prog.Statements[0].(*ast.SetStmt)
	// Should be IndexExpr (dot) of IndexExpr (bracket) of Identifier
	outer, ok := s.Value.(*ast.IndexExpr)
	if !ok {
		t.Fatalf("expected *ast.IndexExpr, got %T", s.Value)
	}
	inner, ok := outer.Object.(*ast.IndexExpr)
	if !ok {
		t.Fatalf("expected nested *ast.IndexExpr, got %T", outer.Object)
	}
	base := inner.Object.(*ast.Identifier)
	if base.Name != "arr" {
		t.Errorf("base: got %q, want %q", base.Name, "arr")
	}
}

func TestEmptyArrayAndDict(t *testing.T) {
	t.Run("empty array", func(t *testing.T) {
		prog := parseSource(t, "SET arr []")
		s := prog.Statements[0].(*ast.SetStmt)
		arr := s.Value.(*ast.ArrayLit)
		if len(arr.Elements) != 0 {
			t.Errorf("elements: got %d, want 0", len(arr.Elements))
		}
	})

	t.Run("empty dict", func(t *testing.T) {
		prog := parseSource(t, "SET d {}")
		s := prog.Statements[0].(*ast.SetStmt)
		dict := s.Value.(*ast.DictLit)
		if len(dict.Keys) != 0 {
			t.Errorf("keys: got %d, want 0", len(dict.Keys))
		}
	})
}

func TestFunctionNoParams(t *testing.T) {
	src := `FUNCTION noop()
    RETURN
ENDFUNCTION`
	prog := parseSource(t, src)
	fn := prog.Statements[0].(*ast.FunctionDef)
	if len(fn.Params) != 0 {
		t.Errorf("params: got %d, want 0", len(fn.Params))
	}
}

func TestMultipleErrorsReported(t *testing.T) {
	// Two bad lines
	src := "@ bad1\n@ bad2"
	tokens, _ := lexer.New(src).Tokenize()
	_, errs := New(tokens).Parse()
	if len(errs) < 2 {
		t.Errorf("expected at least 2 errors, got %d", len(errs))
	}
}
