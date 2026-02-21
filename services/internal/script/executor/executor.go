// Package executor implements the AST walker for the Arturo scripting language.
// It evaluates a parsed AST (produced by the parser) by walking each statement
// and expression node, maintaining scoped variable state, and delegating device
// communication to a DeviceRouter and test result recording to a ResultCollector.
package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/holla2040/arturo/internal/script/ast"
	"github.com/holla2040/arturo/internal/script/token"
	"github.com/holla2040/arturo/internal/script/variable"
)

// ---------------------------------------------------------------------------
// Sentinel errors for control flow
// ---------------------------------------------------------------------------

// ErrBreak signals a BREAK statement inside a loop.
var ErrBreak = errors.New("break")

// ErrContinue signals a CONTINUE statement inside a loop.
var ErrContinue = errors.New("continue")

// ErrTestTerminated signals that PASS, FAIL, or SKIP was called inside a test.
var ErrTestTerminated = errors.New("test terminated")

// ReturnValue wraps a return value for function returns.
type ReturnValue struct {
	Value interface{}
}

func (r *ReturnValue) Error() string { return "return" }

// ---------------------------------------------------------------------------
// Interfaces
// ---------------------------------------------------------------------------

// CommandResult holds the result of a device command.
type CommandResult struct {
	Success    bool
	Response   string
	DurationMs int
}

// DeviceRouter sends commands to devices and returns results.
type DeviceRouter interface {
	SendCommand(ctx context.Context, deviceID, command string, params map[string]string, timeoutMs int) (*CommandResult, error)
}

// ResultCollector records test results during execution.
type ResultCollector interface {
	RecordTestStart(name string)
	RecordTestPass(name, message string)
	RecordTestFail(name, message string)
	RecordTestSkip(name, message string)
	RecordTestError(name, message string)
	RecordAssertion(testName string, passed bool, message string)
	RecordCommand(testName, deviceID, command string, success bool, response string, durationMs int)
	RecordError(message string)
	SetCurrentSuite(name string)
	ClearCurrentSuite()
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

// Option configures the executor.
type Option func(*Executor)

// WithRouter sets the DeviceRouter used to send device commands.
func WithRouter(r DeviceRouter) Option {
	return func(e *Executor) { e.router = r }
}

// WithCollector sets the ResultCollector for recording test results.
func WithCollector(c ResultCollector) Option {
	return func(e *Executor) { e.collector = c }
}

// WithLogger sets the writer for LOG statements.
func WithLogger(w io.Writer) Option {
	return func(e *Executor) { e.logger = w }
}

// ---------------------------------------------------------------------------
// Executor
// ---------------------------------------------------------------------------

// Executor walks an AST and evaluates it.
type Executor struct {
	ctx          context.Context
	env          *variable.Environment
	router       DeviceRouter
	collector    ResultCollector
	logger       io.Writer
	functions    map[string]*ast.FunctionDef
	currentTest  string
	testFinished bool // set when PASS/FAIL/SKIP explicitly ends the current test
}

// New creates a new Executor with the given context and options.
func New(ctx context.Context, opts ...Option) *Executor {
	e := &Executor{
		ctx:       ctx,
		env:       variable.NewEnvironment(),
		functions: make(map[string]*ast.FunctionDef),
		logger:    io.Discard,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Env returns the executor's variable environment (useful for testing).
func (e *Executor) Env() *variable.Environment {
	return e.env
}

// GetVar returns a variable value from the executor's environment. This is
// a convenience method primarily intended for test assertions.
func (e *Executor) GetVar(name string) (interface{}, bool) {
	return e.env.Get(name)
}

// Execute runs the given program by walking its statements.
func (e *Executor) Execute(program *ast.Program) error {
	for _, stmt := range program.Statements {
		if err := e.execStatement(stmt); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Statement dispatch
// ---------------------------------------------------------------------------

func (e *Executor) execStatement(stmt ast.Statement) error {
	switch s := stmt.(type) {
	case *ast.SetStmt:
		return e.execSetStmt(s)
	case *ast.ConstStmt:
		return e.execConstStmt(s)
	case *ast.GlobalStmt:
		return e.execGlobalStmt(s)
	case *ast.DeleteStmt:
		return e.execDeleteStmt(s)
	case *ast.AppendStmt:
		return e.execAppendStmt(s)
	case *ast.ExtendStmt:
		return e.execExtendStmt(s)
	case *ast.IfStmt:
		return e.execIfStmt(s)
	case *ast.LoopStmt:
		return e.execLoopStmt(s)
	case *ast.WhileStmt:
		return e.execWhileStmt(s)
	case *ast.ForEachStmt:
		return e.execForEachStmt(s)
	case *ast.BreakStmt:
		return ErrBreak
	case *ast.ContinueStmt:
		return ErrContinue
	case *ast.TryStmt:
		return e.execTryStmt(s)
	case *ast.ParallelStmt:
		return e.execParallelStmt(s)
	case *ast.ConnectStmt:
		return e.execConnectStmt(s)
	case *ast.DisconnectStmt:
		return e.execDisconnectStmt(s)
	case *ast.SendStmt:
		return e.execSendStmt(s)
	case *ast.QueryStmt:
		return e.execQueryStmt(s)
	case *ast.RelayStmt:
		return e.execRelayStmt(s)
	case *ast.FunctionDef:
		return e.execFunctionDef(s)
	case *ast.ReturnStmt:
		return e.execReturnStmt(s)
	case *ast.ImportStmt:
		return e.execImportStmt(s)
	case *ast.TestDef:
		return e.execTestDef(s)
	case *ast.SuiteDef:
		return e.execSuiteDef(s)
	case *ast.PassStmt:
		return e.execPassStmt(s)
	case *ast.FailStmt:
		return e.execFailStmt(s)
	case *ast.SkipStmt:
		return e.execSkipStmt(s)
	case *ast.AssertStmt:
		return e.execAssertStmt(s)
	case *ast.LogStmt:
		return e.execLogStmt(s)
	case *ast.DelayStmt:
		return e.execDelayStmt(s)
	case *ast.LibraryDef:
		// Execute library body statements in the current scope.
		for _, bs := range s.Body {
			if err := e.execStatement(bs); err != nil {
				return err
			}
		}
		return nil
	case *ast.ReserveStmt:
		// RESERVE allocates a pre-sized array — just create an empty array.
		sizeVal, err := e.evalExpression(s.Size)
		if err != nil {
			return fmt.Errorf("RESERVE: %w", err)
		}
		n, err := variable.ToInt(sizeVal)
		if err != nil {
			return fmt.Errorf("RESERVE: %w", err)
		}
		arr := make([]interface{}, n)
		return e.env.Set(s.Name, arr)
	default:
		return fmt.Errorf("unknown statement type %T", stmt)
	}
}

// ---------------------------------------------------------------------------
// Variable statements
// ---------------------------------------------------------------------------

func (e *Executor) execSetStmt(s *ast.SetStmt) error {
	// Handle standalone CALL (SetStmt with empty name from parser).
	if s.Name == "" {
		if callExpr, ok := s.Value.(*ast.CallExpr); ok {
			_, err := e.execCallExpr(callExpr)
			return err
		}
		// Evaluate the expression for side effects.
		_, err := e.evalExpression(s.Value)
		return err
	}

	val, err := e.evalExpression(s.Value)
	if err != nil {
		return fmt.Errorf("SET %s: %w", s.Name, err)
	}

	// Index assignment: arr[i] = val or dict["key"] = val.
	if s.Index != nil {
		idx, idxErr := e.evalExpression(s.Index)
		if idxErr != nil {
			return fmt.Errorf("SET %s index: %w", s.Name, idxErr)
		}
		obj, ok := e.env.Get(s.Name)
		if !ok {
			return fmt.Errorf("SET %s: variable not found", s.Name)
		}
		switch container := obj.(type) {
		case []interface{}:
			i, convErr := variable.ToInt(idx)
			if convErr != nil {
				return fmt.Errorf("SET %s: array index: %w", s.Name, convErr)
			}
			if i < 0 || int(i) >= len(container) {
				return fmt.Errorf("SET %s: array index %d out of range [0, %d)", s.Name, i, len(container))
			}
			container[i] = val
			return nil
		case map[string]interface{}:
			key := variable.ToString(idx)
			container[key] = val
			return nil
		default:
			return fmt.Errorf("SET %s: cannot index %s", s.Name, variable.TypeName(obj))
		}
	}

	return e.env.Set(s.Name, val)
}

func (e *Executor) execConstStmt(s *ast.ConstStmt) error {
	val, err := e.evalExpression(s.Value)
	if err != nil {
		return fmt.Errorf("CONST %s: %w", s.Name, err)
	}
	return e.env.SetConst(s.Name, val)
}

func (e *Executor) execGlobalStmt(s *ast.GlobalStmt) error {
	var val interface{}
	if s.Value != nil {
		var err error
		val, err = e.evalExpression(s.Value)
		if err != nil {
			return fmt.Errorf("GLOBAL %s: %w", s.Name, err)
		}
	}
	return e.env.SetGlobal(s.Name, val)
}

func (e *Executor) execDeleteStmt(s *ast.DeleteStmt) error {
	return e.env.Delete(s.Name)
}

func (e *Executor) execAppendStmt(s *ast.AppendStmt) error {
	val, err := e.evalExpression(s.Value)
	if err != nil {
		return fmt.Errorf("APPEND %s: %w", s.Name, err)
	}
	obj, ok := e.env.Get(s.Name)
	if !ok {
		return fmt.Errorf("APPEND: variable %q not found", s.Name)
	}
	arr, isArr := obj.([]interface{})
	if !isArr {
		return fmt.Errorf("APPEND: variable %q is not an array", s.Name)
	}
	arr = append(arr, val)
	return e.env.Set(s.Name, arr)
}

func (e *Executor) execExtendStmt(s *ast.ExtendStmt) error {
	val, err := e.evalExpression(s.Value)
	if err != nil {
		return fmt.Errorf("EXTEND %s: %w", s.Name, err)
	}
	obj, ok := e.env.Get(s.Name)
	if !ok {
		return fmt.Errorf("EXTEND: variable %q not found", s.Name)
	}
	arr, isArr := obj.([]interface{})
	if !isArr {
		return fmt.Errorf("EXTEND: variable %q is not an array", s.Name)
	}
	other, isOther := val.([]interface{})
	if !isOther {
		return fmt.Errorf("EXTEND: value is not an array")
	}
	arr = append(arr, other...)
	return e.env.Set(s.Name, arr)
}

// ---------------------------------------------------------------------------
// Control flow
// ---------------------------------------------------------------------------

func (e *Executor) execIfStmt(s *ast.IfStmt) error {
	condVal, err := e.evalExpression(s.Condition)
	if err != nil {
		return fmt.Errorf("IF: %w", err)
	}
	if variable.IsTruthy(condVal) {
		return e.execBlock(s.Body)
	}
	for _, ei := range s.ElseIfs {
		eiVal, eiErr := e.evalExpression(ei.Condition)
		if eiErr != nil {
			return fmt.Errorf("ELSEIF: %w", eiErr)
		}
		if variable.IsTruthy(eiVal) {
			return e.execBlock(ei.Body)
		}
	}
	if len(s.ElseBody) > 0 {
		return e.execBlock(s.ElseBody)
	}
	return nil
}

func (e *Executor) execLoopStmt(s *ast.LoopStmt) error {
	countVal, err := e.evalExpression(s.Count)
	if err != nil {
		return fmt.Errorf("LOOP: %w", err)
	}
	n, err := variable.ToInt(countVal)
	if err != nil {
		return fmt.Errorf("LOOP count: %w", err)
	}

	for i := int64(0); i < n; i++ {
		if s.IterVar != "" {
			if setErr := e.env.Set(s.IterVar, i); setErr != nil {
				return fmt.Errorf("LOOP iter: %w", setErr)
			}
		}
		if blockErr := e.execBlock(s.Body); blockErr != nil {
			if errors.Is(blockErr, ErrBreak) {
				break
			}
			if errors.Is(blockErr, ErrContinue) {
				continue
			}
			return blockErr
		}
	}
	return nil
}

func (e *Executor) execWhileStmt(s *ast.WhileStmt) error {
	for {
		condVal, err := e.evalExpression(s.Condition)
		if err != nil {
			return fmt.Errorf("WHILE: %w", err)
		}
		if !variable.IsTruthy(condVal) {
			break
		}
		if blockErr := e.execBlock(s.Body); blockErr != nil {
			if errors.Is(blockErr, ErrBreak) {
				break
			}
			if errors.Is(blockErr, ErrContinue) {
				continue
			}
			return blockErr
		}
	}
	return nil
}

func (e *Executor) execForEachStmt(s *ast.ForEachStmt) error {
	collVal, err := e.evalExpression(s.Collection)
	if err != nil {
		return fmt.Errorf("FOREACH: %w", err)
	}
	arr, ok := collVal.([]interface{})
	if !ok {
		return fmt.Errorf("FOREACH: collection is not an array, got %s", variable.TypeName(collVal))
	}

	for i, item := range arr {
		if setErr := e.env.Set(s.ItemVar, item); setErr != nil {
			return fmt.Errorf("FOREACH item: %w", setErr)
		}
		if s.IndexVar != "" {
			if setErr := e.env.Set(s.IndexVar, int64(i)); setErr != nil {
				return fmt.Errorf("FOREACH index: %w", setErr)
			}
		}
		if blockErr := e.execBlock(s.Body); blockErr != nil {
			if errors.Is(blockErr, ErrBreak) {
				break
			}
			if errors.Is(blockErr, ErrContinue) {
				continue
			}
			return blockErr
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

func (e *Executor) execTryStmt(s *ast.TryStmt) error {
	bodyErr := e.execBlock(s.Body)

	if bodyErr != nil {
		// Do not catch control flow signals.
		if errors.Is(bodyErr, ErrBreak) || errors.Is(bodyErr, ErrContinue) {
			if len(s.FinallyBody) > 0 {
				_ = e.execBlock(s.FinallyBody)
			}
			return bodyErr
		}
		var rv *ReturnValue
		if errors.As(bodyErr, &rv) {
			if len(s.FinallyBody) > 0 {
				_ = e.execBlock(s.FinallyBody)
			}
			return bodyErr
		}

		// Run catch block.
		if len(s.CatchBody) > 0 {
			if s.CatchVar != "" {
				if setErr := e.env.Set(s.CatchVar, bodyErr.Error()); setErr != nil {
					return setErr
				}
			}
			if catchErr := e.execBlock(s.CatchBody); catchErr != nil {
				if len(s.FinallyBody) > 0 {
					_ = e.execBlock(s.FinallyBody)
				}
				return catchErr
			}
		}
	}

	// Run finally block.
	if len(s.FinallyBody) > 0 {
		if finErr := e.execBlock(s.FinallyBody); finErr != nil {
			return finErr
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Parallel (sequential placeholder)
// ---------------------------------------------------------------------------

func (e *Executor) execParallelStmt(s *ast.ParallelStmt) error {
	// For now, execute statements sequentially.
	return e.execBlock(s.Body)
}

// ---------------------------------------------------------------------------
// Device communication
// ---------------------------------------------------------------------------

func (e *Executor) execConnectStmt(s *ast.ConnectStmt) error {
	addrVal, err := e.evalExpression(s.Address)
	if err != nil {
		return fmt.Errorf("CONNECT: %w", err)
	}
	fmt.Fprintf(e.logger, "CONNECT %s %s %s\n", s.DeviceID, s.Protocol, variable.ToString(addrVal))
	return nil
}

func (e *Executor) execDisconnectStmt(s *ast.DisconnectStmt) error {
	if s.All {
		fmt.Fprintf(e.logger, "DISCONNECT ALL\n")
	} else {
		fmt.Fprintf(e.logger, "DISCONNECT %s\n", s.DeviceID)
	}
	return nil
}

func (e *Executor) execSendStmt(s *ast.SendStmt) error {
	cmdVal, err := e.evalExpression(s.Command)
	if err != nil {
		return fmt.Errorf("SEND: %w", err)
	}
	cmdStr := variable.ToString(cmdVal)

	if e.router == nil {
		fmt.Fprintf(e.logger, "SEND %s (no router)\n", cmdStr)
		return nil
	}

	result, routeErr := e.router.SendCommand(e.ctx, "", cmdStr, nil, 0)
	if routeErr != nil {
		return fmt.Errorf("SEND %s: %w", cmdStr, routeErr)
	}

	if e.collector != nil && e.currentTest != "" {
		e.collector.RecordCommand(e.currentTest, "", cmdStr, result.Success, result.Response, result.DurationMs)
	}

	return nil
}

func (e *Executor) execQueryStmt(s *ast.QueryStmt) error {
	cmdVal, err := e.evalExpression(s.Command)
	if err != nil {
		return fmt.Errorf("QUERY: %w", err)
	}
	cmdStr := variable.ToString(cmdVal)

	timeoutMs := 0
	if s.Timeout != nil {
		tVal, tErr := e.evalExpression(s.Timeout)
		if tErr != nil {
			return fmt.Errorf("QUERY TIMEOUT: %w", tErr)
		}
		t, convErr := variable.ToInt(tVal)
		if convErr != nil {
			return fmt.Errorf("QUERY TIMEOUT: %w", convErr)
		}
		timeoutMs = int(t)
	}

	if e.router == nil {
		fmt.Fprintf(e.logger, "QUERY %s -> %s (no router)\n", cmdStr, s.ResultVar)
		return e.env.Set(s.ResultVar, "")
	}

	result, routeErr := e.router.SendCommand(e.ctx, "", cmdStr, nil, timeoutMs)
	if routeErr != nil {
		return fmt.Errorf("QUERY %s: %w", cmdStr, routeErr)
	}

	if e.collector != nil && e.currentTest != "" {
		e.collector.RecordCommand(e.currentTest, "", cmdStr, result.Success, result.Response, result.DurationMs)
	}

	return e.env.Set(s.ResultVar, result.Response)
}

func (e *Executor) execRelayStmt(s *ast.RelayStmt) error {
	chanVal, err := e.evalExpression(s.Channel)
	if err != nil {
		return fmt.Errorf("RELAY: %w", err)
	}
	chanStr := variable.ToString(chanVal)

	cmd := fmt.Sprintf("RELAY %s %s", s.Action, chanStr)

	if s.State != nil {
		stateVal, stErr := e.evalExpression(s.State)
		if stErr != nil {
			return fmt.Errorf("RELAY state: %w", stErr)
		}
		cmd += " " + variable.ToString(stateVal)
	}

	if e.router == nil {
		fmt.Fprintf(e.logger, "RELAY %s %s (no router)\n", s.DeviceID, cmd)
		return nil
	}

	result, routeErr := e.router.SendCommand(e.ctx, s.DeviceID, cmd, nil, 0)
	if routeErr != nil {
		return fmt.Errorf("RELAY %s: %w", s.DeviceID, routeErr)
	}

	if s.ResultVar != "" {
		if setErr := e.env.Set(s.ResultVar, result.Response); setErr != nil {
			return setErr
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Functions
// ---------------------------------------------------------------------------

func (e *Executor) execFunctionDef(s *ast.FunctionDef) error {
	e.functions[s.Name] = s
	return nil
}

func (e *Executor) execCallExpr(c *ast.CallExpr) (interface{}, error) {
	fn, ok := e.functions[c.Name]
	if !ok {
		return nil, fmt.Errorf("CALL: function %q not defined", c.Name)
	}

	if len(c.Args) != len(fn.Params) {
		return nil, fmt.Errorf("CALL %s: expected %d args, got %d", c.Name, len(fn.Params), len(c.Args))
	}

	// Evaluate arguments before pushing scope.
	argVals := make([]interface{}, len(c.Args))
	for i, argExpr := range c.Args {
		v, err := e.evalExpression(argExpr)
		if err != nil {
			return nil, fmt.Errorf("CALL %s arg %d: %w", c.Name, i, err)
		}
		argVals[i] = v
	}

	// Push function scope (isolated from local scopes, parent is global).
	// PushFunctionScope saves the current scope so recursive calls work.
	e.env.PushFunctionScope()
	defer e.env.PopFunctionScope()

	// Bind parameters using SetLocal so they shadow any same-named vars
	// in parent scopes rather than updating them.
	for i, param := range fn.Params {
		if err := e.env.SetLocal(param, argVals[i]); err != nil {
			return nil, fmt.Errorf("CALL %s param %s: %w", c.Name, param, err)
		}
	}

	// Execute body.
	err := e.execBlock(fn.Body)
	if err != nil {
		var rv *ReturnValue
		if errors.As(err, &rv) {
			return rv.Value, nil
		}
		return nil, err
	}

	return nil, nil
}

func (e *Executor) execReturnStmt(s *ast.ReturnStmt) error {
	var val interface{}
	if s.Value != nil {
		var err error
		val, err = e.evalExpression(s.Value)
		if err != nil {
			return fmt.Errorf("RETURN: %w", err)
		}
	}
	return &ReturnValue{Value: val}
}

func (e *Executor) execImportStmt(s *ast.ImportStmt) error {
	pathVal, err := e.evalExpression(s.Path)
	if err != nil {
		return fmt.Errorf("IMPORT: %w", err)
	}
	fmt.Fprintf(e.logger, "IMPORT %s (not loaded — future work)\n", variable.ToString(pathVal))
	return nil
}

// ---------------------------------------------------------------------------
// Test / Suite
// ---------------------------------------------------------------------------

func (e *Executor) execTestDef(s *ast.TestDef) error {
	nameVal, err := e.evalExpression(s.Name)
	if err != nil {
		return fmt.Errorf("TEST name: %w", err)
	}
	name := variable.ToString(nameVal)

	prevTest := e.currentTest
	prevFinished := e.testFinished
	e.currentTest = name
	e.testFinished = false

	if e.collector != nil {
		e.collector.RecordTestStart(name)
	}

	testErr := e.execBlock(s.Body)

	if testErr != nil {
		// Control flow errors propagate upward.
		if errors.Is(testErr, ErrBreak) || errors.Is(testErr, ErrContinue) {
			e.currentTest = prevTest
			e.testFinished = prevFinished
			return testErr
		}
		var rv *ReturnValue
		if errors.As(testErr, &rv) {
			e.currentTest = prevTest
			e.testFinished = prevFinished
			return testErr
		}

		// Record test error only if not already finished by PASS/FAIL/SKIP.
		if e.collector != nil && !e.testFinished {
			e.collector.RecordTestError(name, testErr.Error())
		}
		e.currentTest = prevTest
		e.testFinished = prevFinished
		return nil
	}

	// If the test finished without an explicit PASS/FAIL/SKIP, mark as passed.
	if e.collector != nil && !e.testFinished {
		e.collector.RecordTestPass(name, "completed")
	}
	e.currentTest = prevTest
	e.testFinished = prevFinished
	return nil
}

func (e *Executor) execSuiteDef(s *ast.SuiteDef) error {
	nameVal, err := e.evalExpression(s.Name)
	if err != nil {
		return fmt.Errorf("SUITE name: %w", err)
	}
	name := variable.ToString(nameVal)

	if e.collector != nil {
		e.collector.SetCurrentSuite(name)
	}

	// Run setup before each test.
	runSetup := func() error {
		if s.Setup != nil {
			return e.execBlock(s.Setup.Body)
		}
		return nil
	}

	// Run teardown after each test.
	runTeardown := func() error {
		if s.Teardown != nil {
			return e.execBlock(s.Teardown.Body)
		}
		return nil
	}

	// Execute body statements (non-test).
	for _, bs := range s.Body {
		if bodyErr := e.execStatement(bs); bodyErr != nil {
			return bodyErr
		}
	}

	// Execute tests with setup/teardown.
	for _, test := range s.Tests {
		if setupErr := runSetup(); setupErr != nil {
			return fmt.Errorf("SUITE %s SETUP: %w", name, setupErr)
		}
		if testErr := e.execTestDef(test); testErr != nil {
			// Run teardown even if test fails.
			_ = runTeardown()
			return testErr
		}
		if teardownErr := runTeardown(); teardownErr != nil {
			return fmt.Errorf("SUITE %s TEARDOWN: %w", name, teardownErr)
		}
	}

	if e.collector != nil {
		e.collector.ClearCurrentSuite()
	}

	return nil
}

// ---------------------------------------------------------------------------
// Test result statements
// ---------------------------------------------------------------------------

func (e *Executor) execPassStmt(s *ast.PassStmt) error {
	msgVal, err := e.evalExpression(s.Message)
	if err != nil {
		return fmt.Errorf("PASS: %w", err)
	}
	msg := variable.ToString(msgVal)
	if e.collector != nil && e.currentTest != "" {
		e.collector.RecordTestPass(e.currentTest, msg)
		e.testFinished = true
	}
	return ErrTestTerminated
}

func (e *Executor) execFailStmt(s *ast.FailStmt) error {
	msgVal, err := e.evalExpression(s.Message)
	if err != nil {
		return fmt.Errorf("FAIL: %w", err)
	}
	msg := variable.ToString(msgVal)
	if e.collector != nil && e.currentTest != "" {
		e.collector.RecordTestFail(e.currentTest, msg)
		e.testFinished = true
	}
	return ErrTestTerminated
}

func (e *Executor) execSkipStmt(s *ast.SkipStmt) error {
	msgVal, err := e.evalExpression(s.Message)
	if err != nil {
		return fmt.Errorf("SKIP: %w", err)
	}
	msg := variable.ToString(msgVal)
	if e.collector != nil && e.currentTest != "" {
		e.collector.RecordTestSkip(e.currentTest, msg)
		e.testFinished = true
	}
	return ErrTestTerminated
}

func (e *Executor) execAssertStmt(s *ast.AssertStmt) error {
	condVal, err := e.evalExpression(s.Condition)
	if err != nil {
		return fmt.Errorf("ASSERT: %w", err)
	}
	passed := variable.IsTruthy(condVal)

	msg := ""
	if s.Message != nil {
		msgVal, msgErr := e.evalExpression(s.Message)
		if msgErr != nil {
			return fmt.Errorf("ASSERT message: %w", msgErr)
		}
		msg = variable.ToString(msgVal)
	}

	if e.collector != nil && e.currentTest != "" {
		e.collector.RecordAssertion(e.currentTest, passed, msg)
	}

	if !passed {
		if e.collector != nil && e.currentTest != "" {
			e.collector.RecordTestFail(e.currentTest, "assertion failed: "+msg)
			e.testFinished = true
		}
		return ErrTestTerminated
	}

	return nil
}

func (e *Executor) execLogStmt(s *ast.LogStmt) error {
	msgVal, err := e.evalExpression(s.Message)
	if err != nil {
		return fmt.Errorf("LOG: %w", err)
	}
	fmt.Fprintf(e.logger, "[%s] %s\n", s.Level, variable.ToString(msgVal))
	return nil
}

func (e *Executor) execDelayStmt(s *ast.DelayStmt) error {
	durVal, err := e.evalExpression(s.Duration)
	if err != nil {
		return fmt.Errorf("DELAY: %w", err)
	}
	ms, convErr := variable.ToInt(durVal)
	if convErr != nil {
		return fmt.Errorf("DELAY: %w", convErr)
	}

	// Context-aware sleep.
	select {
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return nil
	case <-e.ctx.Done():
		return e.ctx.Err()
	}
}

// ---------------------------------------------------------------------------
// Block execution helper
// ---------------------------------------------------------------------------

func (e *Executor) execBlock(stmts []ast.Statement) error {
	for _, stmt := range stmts {
		if err := e.execStatement(stmt); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Expression evaluation
// ---------------------------------------------------------------------------

func (e *Executor) evalExpression(expr ast.Expression) (interface{}, error) {
	switch ex := expr.(type) {
	case *ast.NumberLit:
		if ex.IsFloat {
			f, err := strconv.ParseFloat(ex.Value, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid float literal %q: %w", ex.Value, err)
			}
			return f, nil
		}
		i, err := strconv.ParseInt(ex.Value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid int literal %q: %w", ex.Value, err)
		}
		return i, nil

	case *ast.StringLit:
		return ex.Value, nil

	case *ast.BoolLit:
		return ex.Value, nil

	case *ast.NullLit:
		return nil, nil

	case *ast.Identifier:
		val, ok := e.env.Get(ex.Name)
		if !ok {
			return nil, fmt.Errorf("undefined variable %q", ex.Name)
		}
		return val, nil

	case *ast.BinaryExpr:
		return e.evalBinaryExpr(ex)

	case *ast.UnaryExpr:
		return e.evalUnaryExpr(ex)

	case *ast.IndexExpr:
		return e.evalIndexExpr(ex)

	case *ast.ArrayLit:
		elems := make([]interface{}, len(ex.Elements))
		for i, elem := range ex.Elements {
			v, err := e.evalExpression(elem)
			if err != nil {
				return nil, fmt.Errorf("array element %d: %w", i, err)
			}
			elems[i] = v
		}
		return elems, nil

	case *ast.DictLit:
		m := make(map[string]interface{}, len(ex.Keys))
		for i := range ex.Keys {
			keyVal, err := e.evalExpression(ex.Keys[i])
			if err != nil {
				return nil, fmt.Errorf("dict key %d: %w", i, err)
			}
			valVal, err := e.evalExpression(ex.Values[i])
			if err != nil {
				return nil, fmt.Errorf("dict value %d: %w", i, err)
			}
			m[variable.ToString(keyVal)] = valVal
		}
		return m, nil

	case *ast.BuiltinCallExpr:
		// Special case: EXISTS — catch identifier resolution errors.
		if ex.Name == "EXISTS" && len(ex.Args) == 1 {
			_, err := e.evalExpression(ex.Args[0])
			return err == nil, nil
		}
		args := make([]interface{}, len(ex.Args))
		for i, argExpr := range ex.Args {
			v, err := e.evalExpression(argExpr)
			if err != nil {
				return nil, fmt.Errorf("builtin %s arg %d: %w", ex.Name, i, err)
			}
			args[i] = v
		}
		return e.evalBuiltin(ex.Name, args)

	case *ast.CallExpr:
		return e.execCallExpr(ex)

	default:
		return nil, fmt.Errorf("unknown expression type %T", expr)
	}
}

func (e *Executor) evalBinaryExpr(ex *ast.BinaryExpr) (interface{}, error) {
	// Short-circuit for AND/OR.
	if ex.Op == token.TOKEN_AND {
		left, err := e.evalExpression(ex.Left)
		if err != nil {
			return nil, err
		}
		if !variable.IsTruthy(left) {
			return false, nil
		}
		right, err := e.evalExpression(ex.Right)
		if err != nil {
			return nil, err
		}
		return variable.IsTruthy(right), nil
	}
	if ex.Op == token.TOKEN_OR {
		left, err := e.evalExpression(ex.Left)
		if err != nil {
			return nil, err
		}
		if variable.IsTruthy(left) {
			return true, nil
		}
		right, err := e.evalExpression(ex.Right)
		if err != nil {
			return nil, err
		}
		return variable.IsTruthy(right), nil
	}

	left, err := e.evalExpression(ex.Left)
	if err != nil {
		return nil, err
	}
	right, err := e.evalExpression(ex.Right)
	if err != nil {
		return nil, err
	}

	switch ex.Op {
	case token.TOKEN_PLUS:
		return variable.Add(left, right)
	case token.TOKEN_MINUS:
		return variable.Subtract(left, right)
	case token.TOKEN_STAR:
		return variable.Multiply(left, right)
	case token.TOKEN_SLASH:
		return variable.Divide(left, right)
	case token.TOKEN_PERCENT:
		return variable.Modulo(left, right)
	case token.TOKEN_EQ:
		return variable.Equal(left, right), nil
	case token.TOKEN_NEQ:
		return !variable.Equal(left, right), nil
	case token.TOKEN_GT:
		cmp, cmpErr := variable.Compare(left, right)
		if cmpErr != nil {
			return nil, cmpErr
		}
		return cmp > 0, nil
	case token.TOKEN_LT:
		cmp, cmpErr := variable.Compare(left, right)
		if cmpErr != nil {
			return nil, cmpErr
		}
		return cmp < 0, nil
	case token.TOKEN_GTE:
		cmp, cmpErr := variable.Compare(left, right)
		if cmpErr != nil {
			return nil, cmpErr
		}
		return cmp >= 0, nil
	case token.TOKEN_LTE:
		cmp, cmpErr := variable.Compare(left, right)
		if cmpErr != nil {
			return nil, cmpErr
		}
		return cmp <= 0, nil
	default:
		return nil, fmt.Errorf("unknown binary operator %s", ex.Op)
	}
}

func (e *Executor) evalUnaryExpr(ex *ast.UnaryExpr) (interface{}, error) {
	val, err := e.evalExpression(ex.Operand)
	if err != nil {
		return nil, err
	}

	switch ex.Op {
	case token.TOKEN_NOT:
		return !variable.ToBool(val), nil
	case token.TOKEN_MINUS:
		return variable.Negate(val)
	default:
		return nil, fmt.Errorf("unknown unary operator %s", ex.Op)
	}
}

func (e *Executor) evalIndexExpr(ex *ast.IndexExpr) (interface{}, error) {
	obj, err := e.evalExpression(ex.Object)
	if err != nil {
		return nil, err
	}
	idx, err := e.evalExpression(ex.Index)
	if err != nil {
		return nil, err
	}

	switch container := obj.(type) {
	case []interface{}:
		i, convErr := variable.ToInt(idx)
		if convErr != nil {
			return nil, fmt.Errorf("array index: %w", convErr)
		}
		if i < 0 || int(i) >= len(container) {
			return nil, fmt.Errorf("array index %d out of range [0, %d)", i, len(container))
		}
		return container[i], nil
	case map[string]interface{}:
		key := variable.ToString(idx)
		val, ok := container[key]
		if !ok {
			return nil, nil
		}
		return val, nil
	default:
		return nil, fmt.Errorf("cannot index %s", variable.TypeName(obj))
	}
}
