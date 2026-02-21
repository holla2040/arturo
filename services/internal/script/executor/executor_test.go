package executor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/holla2040/arturo/internal/script/lexer"
	"github.com/holla2040/arturo/internal/script/parser"
)

// ---------------------------------------------------------------------------
// Mock types
// ---------------------------------------------------------------------------

type recordedCommand struct {
	deviceID  string
	command   string
	timeoutMs int
}

type mockRouter struct {
	commands []recordedCommand
	response *CommandResult
	err      error
}

func (m *mockRouter) SendCommand(_ context.Context, deviceID, command string, _ map[string]string, timeoutMs int) (*CommandResult, error) {
	m.commands = append(m.commands, recordedCommand{deviceID, command, timeoutMs})
	if m.err != nil {
		return nil, m.err
	}
	if m.response != nil {
		return m.response, nil
	}
	return &CommandResult{Success: true, Response: "OK", DurationMs: 1}, nil
}

type assertionRecord struct {
	testName string
	passed   bool
	message  string
}

type commandRecord struct {
	testName  string
	deviceID  string
	command   string
	success   bool
	response  string
	durationMs int
}

type errorRecord struct {
	message string
}

type mockCollector struct {
	testStarts   []string
	testPasses   []string
	testFails    []string
	testSkips    []string
	testErrors   []string
	assertions   []assertionRecord
	commands     []commandRecord
	errors       []errorRecord
	currentSuite string
}

func (m *mockCollector) RecordTestStart(name string)  { m.testStarts = append(m.testStarts, name) }
func (m *mockCollector) RecordTestPass(name, _ string) { m.testPasses = append(m.testPasses, name) }
func (m *mockCollector) RecordTestFail(name, _ string) { m.testFails = append(m.testFails, name) }
func (m *mockCollector) RecordTestSkip(name, _ string) { m.testSkips = append(m.testSkips, name) }
func (m *mockCollector) RecordTestError(name, _ string) {
	m.testErrors = append(m.testErrors, name)
}
func (m *mockCollector) RecordAssertion(testName string, passed bool, message string) {
	m.assertions = append(m.assertions, assertionRecord{testName, passed, message})
}
func (m *mockCollector) RecordCommand(testName, deviceID, command string, success bool, response string, durationMs int) {
	m.commands = append(m.commands, commandRecord{testName, deviceID, command, success, response, durationMs})
}
func (m *mockCollector) RecordError(message string) {
	m.errors = append(m.errors, errorRecord{message})
}
func (m *mockCollector) SetCurrentSuite(name string) { m.currentSuite = name }
func (m *mockCollector) ClearCurrentSuite()           { m.currentSuite = "" }

// ---------------------------------------------------------------------------
// Test helper
// ---------------------------------------------------------------------------

func parseAndExec(t *testing.T, source string, opts ...Option) (*Executor, error) {
	t.Helper()
	tokens, lexErrs := lexer.New(source).Tokenize()
	if len(lexErrs) > 0 {
		t.Fatalf("lex errors: %v", lexErrs)
	}
	prog, parseErrs := parser.New(tokens).Parse()
	if len(parseErrs) > 0 {
		t.Fatalf("parse errors: %v", parseErrs)
	}
	exec := New(context.Background(), opts...)
	err := exec.Execute(prog)
	return exec, err
}

// ---------------------------------------------------------------------------
// Variables & expressions
// ---------------------------------------------------------------------------

func TestVariablesAndExpressions(t *testing.T) {
	tests := []struct {
		name   string
		source string
		check  func(t *testing.T, e *Executor)
	}{
		{
			name:   "SET integer",
			source: "SET x 42",
			check: func(t *testing.T, e *Executor) {
				v, ok := e.Env().Get("x")
				if !ok {
					t.Fatal("x not found")
				}
				if v != int64(42) {
					t.Fatalf("expected 42, got %v", v)
				}
			},
		},
		{
			name:   "SET float",
			source: "SET pi 3.14",
			check: func(t *testing.T, e *Executor) {
				v, ok := e.Env().Get("pi")
				if !ok {
					t.Fatal("pi not found")
				}
				if v != 3.14 {
					t.Fatalf("expected 3.14, got %v", v)
				}
			},
		},
		{
			name:   "SET string",
			source: `SET name "hello"`,
			check: func(t *testing.T, e *Executor) {
				v, ok := e.Env().Get("name")
				if !ok {
					t.Fatal("name not found")
				}
				if v != "hello" {
					t.Fatalf("expected hello, got %v", v)
				}
			},
		},
		{
			name:   "SET with expression",
			source: "SET total 2 + 3",
			check: func(t *testing.T, e *Executor) {
				v, ok := e.Env().Get("total")
				if !ok {
					t.Fatal("total not found")
				}
				if v != int64(5) {
					t.Fatalf("expected 5, got %v", v)
				}
			},
		},
		{
			name:   "SET with complex expression (precedence)",
			source: "SET result 2 + 3 * 4",
			check: func(t *testing.T, e *Executor) {
				v, ok := e.Env().Get("result")
				if !ok {
					t.Fatal("result not found")
				}
				if v != int64(14) {
					t.Fatalf("expected 14 (2 + 3*4), got %v", v)
				}
			},
		},
		{
			name:   "CONST immutable",
			source: "CONST MAX 100",
			check: func(t *testing.T, e *Executor) {
				v, ok := e.Env().Get("MAX")
				if !ok {
					t.Fatal("MAX not found")
				}
				if v != int64(100) {
					t.Fatalf("expected 100, got %v", v)
				}
				err := e.Env().Set("MAX", int64(200))
				if err == nil {
					t.Fatal("expected error when assigning to constant")
				}
			},
		},
		{
			name:   "Array literal",
			source: "SET arr [1, 2, 3]",
			check: func(t *testing.T, e *Executor) {
				v, ok := e.Env().Get("arr")
				if !ok {
					t.Fatal("arr not found")
				}
				arr, isArr := v.([]interface{})
				if !isArr {
					t.Fatalf("expected array, got %T", v)
				}
				if len(arr) != 3 {
					t.Fatalf("expected 3 elements, got %d", len(arr))
				}
				if arr[0] != int64(1) || arr[1] != int64(2) || arr[2] != int64(3) {
					t.Fatalf("unexpected array values: %v", arr)
				}
			},
		},
		{
			name:   "Dict literal",
			source: `SET d {"key": "val"}`,
			check: func(t *testing.T, e *Executor) {
				v, ok := e.Env().Get("d")
				if !ok {
					t.Fatal("d not found")
				}
				m, isMap := v.(map[string]interface{})
				if !isMap {
					t.Fatalf("expected map, got %T", v)
				}
				if m["key"] != "val" {
					t.Fatalf("expected val, got %v", m["key"])
				}
			},
		},
		{
			name: "Index access",
			source: `SET arr [10, 20, 30]
SET x arr[1]`,
			check: func(t *testing.T, e *Executor) {
				v, ok := e.Env().Get("x")
				if !ok {
					t.Fatal("x not found")
				}
				if v != int64(20) {
					t.Fatalf("expected 20, got %v", v)
				}
			},
		},
		{
			name:   "String concatenation",
			source: `SET msg "hello" + " " + "world"`,
			check: func(t *testing.T, e *Executor) {
				v, ok := e.Env().Get("msg")
				if !ok {
					t.Fatal("msg not found")
				}
				if v != "hello world" {
					t.Fatalf("expected 'hello world', got %v", v)
				}
			},
		},
		{
			name:   "Boolean AND",
			source: "SET x TRUE && FALSE",
			check: func(t *testing.T, e *Executor) {
				v, ok := e.Env().Get("x")
				if !ok {
					t.Fatal("x not found")
				}
				if v != false {
					t.Fatalf("expected false, got %v", v)
				}
			},
		},
		{
			name: "Unary operations",
			source: `SET x -5
SET y !TRUE`,
			check: func(t *testing.T, e *Executor) {
				x, ok := e.Env().Get("x")
				if !ok {
					t.Fatal("x not found")
				}
				if x != int64(-5) {
					t.Fatalf("expected -5, got %v", x)
				}
				y, ok := e.Env().Get("y")
				if !ok {
					t.Fatal("y not found")
				}
				if y != false {
					t.Fatalf("expected false, got %v", y)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exec, err := parseAndExec(t, tc.source)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, exec)
		})
	}
}

// ---------------------------------------------------------------------------
// Control flow
// ---------------------------------------------------------------------------

func TestControlFlow(t *testing.T) {
	t.Run("IF true", func(t *testing.T) {
		src := `SET x 0
IF TRUE
  SET x 1
ENDIF`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("x")
		if v != int64(1) {
			t.Fatalf("expected 1, got %v", v)
		}
	})

	t.Run("IF false", func(t *testing.T) {
		src := `SET x 0
IF FALSE
  SET x 1
ENDIF`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("x")
		if v != int64(0) {
			t.Fatalf("expected 0, got %v", v)
		}
	})

	t.Run("IF/ELSE", func(t *testing.T) {
		src := `SET x 0
IF FALSE
  SET x 1
ELSE
  SET x 2
ENDIF`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("x")
		if v != int64(2) {
			t.Fatalf("expected 2, got %v", v)
		}
	})

	t.Run("IF/ELSEIF/ELSE", func(t *testing.T) {
		src := `SET x 0
IF FALSE
  SET x 1
ELSEIF TRUE
  SET x 3
ELSE
  SET x 2
ENDIF`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("x")
		if v != int64(3) {
			t.Fatalf("expected 3, got %v", v)
		}
	})

	t.Run("LOOP N TIMES", func(t *testing.T) {
		src := `SET count 0
LOOP 5 TIMES
  SET count count + 1
ENDLOOP`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("count")
		if v != int64(5) {
			t.Fatalf("expected 5, got %v", v)
		}
	})

	t.Run("LOOP with AS", func(t *testing.T) {
		src := `SET sum 0
LOOP 4 TIMES AS i
  SET sum sum + i
ENDLOOP`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("sum")
		// 0+1+2+3 = 6
		if v != int64(6) {
			t.Fatalf("expected 6, got %v", v)
		}
	})

	t.Run("WHILE", func(t *testing.T) {
		src := `SET x 10
WHILE x > 0
  SET x x - 3
ENDWHILE`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("x")
		// 10 -> 7 -> 4 -> 1 -> -2
		if v != int64(-2) {
			t.Fatalf("expected -2, got %v", v)
		}
	})

	t.Run("FOREACH", func(t *testing.T) {
		src := `SET arr [10, 20, 30]
SET sum 0
FOREACH item IN arr
  SET sum sum + item
ENDFOREACH`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("sum")
		if v != int64(60) {
			t.Fatalf("expected 60, got %v", v)
		}
	})

	t.Run("BREAK", func(t *testing.T) {
		src := `SET x 0
LOOP 100 TIMES
  SET x x + 1
  IF x == 3
    BREAK
  ENDIF
ENDLOOP`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("x")
		if v != int64(3) {
			t.Fatalf("expected 3, got %v", v)
		}
	})

	t.Run("CONTINUE", func(t *testing.T) {
		src := `SET sum 0
LOOP 5 TIMES AS i
  IF i == 2
    CONTINUE
  ENDIF
  SET sum sum + i
ENDLOOP`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("sum")
		// 0+1+3+4 = 8 (skipping i=2)
		if v != int64(8) {
			t.Fatalf("expected 8, got %v", v)
		}
	})
}

// ---------------------------------------------------------------------------
// Functions
// ---------------------------------------------------------------------------

func TestFunctions(t *testing.T) {
	t.Run("definition and call", func(t *testing.T) {
		src := `FUNCTION greet(name)
  SET result "hello " + name
ENDFUNCTION
SET x CALL greet("world")`
		// The function sets result in its own scope, but CALL returns nil
		// since there's no RETURN. We need RETURN for value.
		// Actually, let's test with RETURN.
		src = `FUNCTION greet(name)
  RETURN "hello " + name
ENDFUNCTION
SET x CALL greet("world")`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("x")
		if v != "hello world" {
			t.Fatalf("expected 'hello world', got %v", v)
		}
	})

	t.Run("CALL with return value", func(t *testing.T) {
		src := `FUNCTION add(a, b)
  RETURN a + b
ENDFUNCTION
SET result CALL add(1, 2)`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("result")
		if v != int64(3) {
			t.Fatalf("expected 3, got %v", v)
		}
	})

	t.Run("function scope isolation", func(t *testing.T) {
		src := `SET localvar 42
FUNCTION getlocal()
  RETURN localvar
ENDFUNCTION
SET result CALL getlocal()`
		// localvar is in the "global" scope (the root scope), so the function
		// should see it via function scope (parent = global).
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("result")
		if v != int64(42) {
			t.Fatalf("expected 42, got %v", v)
		}
	})

	t.Run("function can see globals", func(t *testing.T) {
		src := `GLOBAL gval 99
FUNCTION readglobal()
  RETURN gval
ENDFUNCTION
SET result CALL readglobal()`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("result")
		if v != int64(99) {
			t.Fatalf("expected 99, got %v", v)
		}
	})

	t.Run("recursive function (factorial)", func(t *testing.T) {
		src := `FUNCTION fact(n)
  IF n <= 1
    RETURN 1
  ENDIF
  SET sub CALL fact(n - 1)
  RETURN n * sub
ENDFUNCTION
SET result CALL fact(5)`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("result")
		if v != int64(120) {
			t.Fatalf("expected 120, got %v", v)
		}
	})

	t.Run("RETURN from function", func(t *testing.T) {
		src := `FUNCTION early()
  RETURN 10
  SET x 20
ENDFUNCTION
SET result CALL early()`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("result")
		if v != int64(10) {
			t.Fatalf("expected 10, got %v", v)
		}
	})
}

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

func TestErrorHandling(t *testing.T) {
	t.Run("TRY/CATCH runs catch on error", func(t *testing.T) {
		src := `SET caught ""
TRY
  SET x undefined_var + 1
CATCH e
  SET caught e
ENDTRY`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("caught")
		s, ok := v.(string)
		if !ok || s == "" {
			t.Fatalf("expected non-empty caught error, got %v", v)
		}
	})

	t.Run("TRY without error skips catch", func(t *testing.T) {
		src := `SET caught ""
SET x 42
TRY
  SET y x + 1
CATCH e
  SET caught "error"
ENDTRY`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("caught")
		if v != "" {
			t.Fatalf("expected empty caught, got %v", v)
		}
		y, _ := exec.Env().Get("y")
		if y != int64(43) {
			t.Fatalf("expected 43, got %v", y)
		}
	})

	t.Run("TRY/CATCH/FINALLY always runs finally", func(t *testing.T) {
		src := `SET finally_ran FALSE
TRY
  SET x undefined_var + 1
CATCH e
  SET caught TRUE
FINALLY
  SET finally_ran TRUE
ENDTRY`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("finally_ran")
		if v != true {
			t.Fatalf("expected finally_ran=true, got %v", v)
		}
	})

	t.Run("nested TRY", func(t *testing.T) {
		src := `SET outer_caught ""
SET inner_caught ""
TRY
  TRY
    SET x unknown + 1
  CATCH ie
    SET inner_caught ie
  ENDTRY
  SET y also_unknown + 1
CATCH oe
  SET outer_caught oe
ENDTRY`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		inner, _ := exec.Env().Get("inner_caught")
		outer, _ := exec.Env().Get("outer_caught")
		if inner == "" {
			t.Fatal("expected inner_caught to be non-empty")
		}
		if outer == "" {
			t.Fatal("expected outer_caught to be non-empty")
		}
	})
}

// ---------------------------------------------------------------------------
// Device commands
// ---------------------------------------------------------------------------

func TestDeviceCommands(t *testing.T) {
	t.Run("SEND routes to mock router", func(t *testing.T) {
		router := &mockRouter{}
		src := `SEND "pump_on"`
		_, err := parseAndExec(t, src, WithRouter(router))
		if err != nil {
			t.Fatal(err)
		}
		if len(router.commands) != 1 {
			t.Fatalf("expected 1 command, got %d", len(router.commands))
		}
		if router.commands[0].command != "pump_on" {
			t.Fatalf("expected command pump_on, got %s", router.commands[0].command)
		}
	})

	t.Run("QUERY routes and stores response", func(t *testing.T) {
		router := &mockRouter{
			response: &CommandResult{Success: true, Response: "65.3", DurationMs: 5},
		}
		src := `QUERY "get_temp_1st_stage" t1`
		exec, err := parseAndExec(t, src, WithRouter(router))
		if err != nil {
			t.Fatal(err)
		}
		v, ok := exec.Env().Get("t1")
		if !ok {
			t.Fatal("t1 not found")
		}
		if v != "65.3" {
			t.Fatalf("expected 65.3, got %v", v)
		}
	})

	t.Run("QUERY with TIMEOUT", func(t *testing.T) {
		router := &mockRouter{}
		src := `QUERY "pump_status" status TIMEOUT 5000`
		_, err := parseAndExec(t, src, WithRouter(router))
		if err != nil {
			t.Fatal(err)
		}
		if len(router.commands) != 1 {
			t.Fatalf("expected 1 command, got %d", len(router.commands))
		}
		if router.commands[0].timeoutMs != 5000 {
			t.Fatalf("expected timeout 5000, got %d", router.commands[0].timeoutMs)
		}
	})

	t.Run("multiple SEND commands", func(t *testing.T) {
		router := &mockRouter{}
		src := `SEND "pump_on"
SEND "start_regen"
SEND "pump_off"`
		_, err := parseAndExec(t, src, WithRouter(router))
		if err != nil {
			t.Fatal(err)
		}
		if len(router.commands) != 3 {
			t.Fatalf("expected 3 commands, got %d", len(router.commands))
		}
	})

	t.Run("router error propagates", func(t *testing.T) {
		router := &mockRouter{
			err: errors.New("connection refused"),
		}
		src := `SEND "pump_on"`
		_, err := parseAndExec(t, src, WithRouter(router))
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "connection refused") {
			t.Fatalf("expected 'connection refused' in error, got %v", err)
		}
	})

	t.Run("CONNECT/DISCONNECT logged", func(t *testing.T) {
		var buf bytes.Buffer
		src := `CONNECT dmm TCP "192.168.1.100"
DISCONNECT dmm`
		_, err := parseAndExec(t, src, WithLogger(&buf))
		if err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "CONNECT dmm TCP") {
			t.Fatalf("expected CONNECT log, got: %s", out)
		}
		if !strings.Contains(out, "DISCONNECT dmm") {
			t.Fatalf("expected DISCONNECT log, got: %s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// Test results
// ---------------------------------------------------------------------------

func TestTestResults(t *testing.T) {
	t.Run("TEST block pass", func(t *testing.T) {
		coll := &mockCollector{}
		src := `TEST "voltage check"
  SET x 42
ENDTEST`
		_, err := parseAndExec(t, src, WithCollector(coll))
		if err != nil {
			t.Fatal(err)
		}
		if len(coll.testStarts) != 1 || coll.testStarts[0] != "voltage check" {
			t.Fatalf("expected test start, got %v", coll.testStarts)
		}
		if len(coll.testPasses) != 1 {
			t.Fatalf("expected 1 pass, got %d", len(coll.testPasses))
		}
	})

	t.Run("TEST block fail", func(t *testing.T) {
		coll := &mockCollector{}
		src := `TEST "bad test"
  FAIL "something went wrong"
ENDTEST`
		_, err := parseAndExec(t, src, WithCollector(coll))
		if err != nil {
			t.Fatal(err)
		}
		if len(coll.testFails) != 1 {
			t.Fatalf("expected 1 fail, got %d", len(coll.testFails))
		}
	})

	t.Run("ASSERT true passes", func(t *testing.T) {
		coll := &mockCollector{}
		src := `TEST "assert test"
  ASSERT TRUE "should pass"
ENDTEST`
		_, err := parseAndExec(t, src, WithCollector(coll))
		if err != nil {
			t.Fatal(err)
		}
		if len(coll.assertions) != 1 {
			t.Fatalf("expected 1 assertion, got %d", len(coll.assertions))
		}
		if !coll.assertions[0].passed {
			t.Fatal("expected assertion to pass")
		}
	})

	t.Run("ASSERT false fails test", func(t *testing.T) {
		coll := &mockCollector{}
		src := `TEST "assert fail"
  ASSERT FALSE "this fails"
ENDTEST`
		_, err := parseAndExec(t, src, WithCollector(coll))
		if err != nil {
			t.Fatal(err)
		}
		if len(coll.assertions) != 1 {
			t.Fatalf("expected 1 assertion, got %d", len(coll.assertions))
		}
		if coll.assertions[0].passed {
			t.Fatal("expected assertion to fail")
		}
		// The test should be recorded as failed (assertion failure records test fail).
		if len(coll.testFails) != 1 {
			t.Fatalf("expected 1 test fail, got %d", len(coll.testFails))
		}
	})

	t.Run("SUITE with SETUP/TEARDOWN", func(t *testing.T) {
		coll := &mockCollector{}
		src := `SUITE "power tests"
  SETUP
    SET voltage 0
  ENDSETUP

  TEARDOWN
    SET voltage 0
  ENDTEARDOWN

  TEST "test1"
    SET x 1
  ENDTEST

  TEST "test2"
    SET y 2
  ENDTEST
ENDSUITE`
		_, err := parseAndExec(t, src, WithCollector(coll))
		if err != nil {
			t.Fatal(err)
		}
		if coll.currentSuite != "" {
			t.Fatalf("expected suite to be cleared, got %q", coll.currentSuite)
		}
		if len(coll.testStarts) != 2 {
			t.Fatalf("expected 2 test starts, got %d", len(coll.testStarts))
		}
	})

	t.Run("LOG writes to logger", func(t *testing.T) {
		var buf bytes.Buffer
		src := `LOG INFO "system ready"`
		_, err := parseAndExec(t, src, WithLogger(&buf))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "[INFO] system ready") {
			t.Fatalf("expected log output, got: %s", buf.String())
		}
	})

	t.Run("DELAY evaluates duration", func(t *testing.T) {
		src := `DELAY 1`
		_, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("SKIP marks test as skipped", func(t *testing.T) {
		coll := &mockCollector{}
		src := `TEST "skipped test"
  SKIP "not ready"
ENDTEST`
		_, err := parseAndExec(t, src, WithCollector(coll))
		if err != nil {
			t.Fatal(err)
		}
		if len(coll.testSkips) != 1 {
			t.Fatalf("expected 1 skip, got %d", len(coll.testSkips))
		}
	})
}

// ---------------------------------------------------------------------------
// Builtins
// ---------------------------------------------------------------------------

func TestBuiltins(t *testing.T) {
	t.Run("FLOAT converts string to float", func(t *testing.T) {
		src := `SET x FLOAT("3.14")`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("x")
		if v != 3.14 {
			t.Fatalf("expected 3.14, got %v", v)
		}
	})

	t.Run("STRING converts number to string", func(t *testing.T) {
		src := `SET x STRING(42)`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("x")
		if v != "42" {
			t.Fatalf("expected '42', got %v", v)
		}
	})

	t.Run("LENGTH returns array length", func(t *testing.T) {
		src := `SET arr [1, 2, 3, 4, 5]
SET n LENGTH(arr)`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("n")
		if v != int64(5) {
			t.Fatalf("expected 5, got %v", v)
		}
	})

	t.Run("TYPE returns type name", func(t *testing.T) {
		src := `SET t TYPE(42)`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("t")
		if v != "int" {
			t.Fatalf("expected 'int', got %v", v)
		}
	})

	t.Run("NOW returns non-empty string", func(t *testing.T) {
		src := `SET ts NOW()`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := exec.Env().Get("ts")
		s, ok := v.(string)
		if !ok || s == "" {
			t.Fatalf("expected non-empty timestamp string, got %v", v)
		}
	})

	t.Run("BOOL converts various types", func(t *testing.T) {
		src := `SET a BOOL(1)
SET b BOOL(0)
SET c BOOL("hello")
SET d BOOL("")`
		exec, err := parseAndExec(t, src)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := exec.Env().Get("a")
		if a != true {
			t.Fatalf("expected BOOL(1)=true, got %v", a)
		}
		b, _ := exec.Env().Get("b")
		if b != false {
			t.Fatalf("expected BOOL(0)=false, got %v", b)
		}
		c, _ := exec.Env().Get("c")
		if c != true {
			t.Fatalf("expected BOOL('hello')=true, got %v", c)
		}
		d, _ := exec.Env().Get("d")
		if d != false {
			t.Fatalf("expected BOOL('')=false, got %v", d)
		}
	})
}
