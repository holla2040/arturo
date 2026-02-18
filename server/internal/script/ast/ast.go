// Package ast defines the abstract syntax tree node types for the Arturo
// scripting language (.art files).
package ast

import "github.com/holla2040/arturo/internal/script/token"

// ---------------------------------------------------------------------------
// Interfaces
// ---------------------------------------------------------------------------

// Node is the common interface for every AST node.
type Node interface {
	Pos() token.Position
}

// Statement is a node that represents a statement.
type Statement interface {
	Node
	stmtNode()
}

// Expression is a node that represents an expression.
type Expression interface {
	Node
	exprNode()
}

// ---------------------------------------------------------------------------
// Program (root)
// ---------------------------------------------------------------------------

// Program is the top-level AST node representing an entire .art file.
type Program struct {
	Statements []Statement
	Position   token.Position
}

func (p *Program) Pos() token.Position { return p.Position }

// ---------------------------------------------------------------------------
// Test / Suite structure
// ---------------------------------------------------------------------------

// TestDef represents a TEST ... ENDTEST block.
type TestDef struct {
	Name     Expression  // string expression for the test name
	Body     []Statement // statements inside the test
	Position token.Position
}

func (n *TestDef) Pos() token.Position { return n.Position }
func (n *TestDef) stmtNode()           {}

// SuiteDef represents a SUITE ... ENDSUITE block.
type SuiteDef struct {
	Name     Expression      // string expression for the suite name
	Setup    *SetupBlock     // optional SETUP block
	Teardown *TeardownBlock  // optional TEARDOWN block
	Tests    []*TestDef      // test definitions inside the suite
	Body     []Statement     // non-test statements inside the suite
	Position token.Position
}

func (n *SuiteDef) Pos() token.Position { return n.Position }
func (n *SuiteDef) stmtNode()           {}

// SetupBlock represents a SETUP ... ENDSETUP block inside a suite.
type SetupBlock struct {
	Body     []Statement
	Position token.Position
}

func (n *SetupBlock) Pos() token.Position { return n.Position }

// TeardownBlock represents a TEARDOWN ... ENDTEARDOWN block inside a suite.
type TeardownBlock struct {
	Body     []Statement
	Position token.Position
}

func (n *TeardownBlock) Pos() token.Position { return n.Position }

// ---------------------------------------------------------------------------
// Variable statements
// ---------------------------------------------------------------------------

// SetStmt represents SET name [= ] value or SET name[index] [= ] value.
type SetStmt struct {
	Name     string         // variable name
	Index    Expression     // may be nil for simple SET
	Value    Expression     // the value expression
	Position token.Position
}

func (n *SetStmt) Pos() token.Position { return n.Position }
func (n *SetStmt) stmtNode()           {}

// ConstStmt represents CONST name value.
type ConstStmt struct {
	Name     string
	Value    Expression
	Position token.Position
}

func (n *ConstStmt) Pos() token.Position { return n.Position }
func (n *ConstStmt) stmtNode()           {}

// GlobalStmt represents GLOBAL name [value].
type GlobalStmt struct {
	Name     string
	Value    Expression // may be nil
	Position token.Position
}

func (n *GlobalStmt) Pos() token.Position { return n.Position }
func (n *GlobalStmt) stmtNode()           {}

// DeleteStmt represents DELETE name.
type DeleteStmt struct {
	Name     string
	Position token.Position
}

func (n *DeleteStmt) Pos() token.Position { return n.Position }
func (n *DeleteStmt) stmtNode()           {}

// AppendStmt represents APPEND name value.
type AppendStmt struct {
	Name     string
	Value    Expression
	Position token.Position
}

func (n *AppendStmt) Pos() token.Position { return n.Position }
func (n *AppendStmt) stmtNode()           {}

// ExtendStmt represents EXTEND name value.
type ExtendStmt struct {
	Name     string
	Value    Expression
	Position token.Position
}

func (n *ExtendStmt) Pos() token.Position { return n.Position }
func (n *ExtendStmt) stmtNode()           {}

// ---------------------------------------------------------------------------
// Control flow
// ---------------------------------------------------------------------------

// IfStmt represents IF ... [ELSEIF ... ] [ELSE ... ] ENDIF.
type IfStmt struct {
	Condition Expression
	Body      []Statement
	ElseIfs   []ElseIfClause
	ElseBody  []Statement // may be nil
	Position  token.Position
}

func (n *IfStmt) Pos() token.Position { return n.Position }
func (n *IfStmt) stmtNode()           {}

// ElseIfClause is a single ELSEIF branch (not a Statement itself).
type ElseIfClause struct {
	Condition Expression
	Body      []Statement
	Position  token.Position
}

func (n *ElseIfClause) Pos() token.Position { return n.Position }

// LoopStmt represents LOOP count TIMES [AS iterVar] ... ENDLOOP.
type LoopStmt struct {
	Count    Expression // number of iterations
	IterVar  string     // may be empty (from AS clause)
	Body     []Statement
	Position token.Position
}

func (n *LoopStmt) Pos() token.Position { return n.Position }
func (n *LoopStmt) stmtNode()           {}

// WhileStmt represents WHILE condition ... ENDWHILE.
type WhileStmt struct {
	Condition Expression
	Body      []Statement
	Position  token.Position
}

func (n *WhileStmt) Pos() token.Position { return n.Position }
func (n *WhileStmt) stmtNode()           {}

// ForEachStmt represents FOREACH itemVar IN collection [AS indexVar] ... ENDFOREACH.
type ForEachStmt struct {
	ItemVar    string     // the iteration variable
	Collection Expression // the collection to iterate
	IndexVar   string     // may be empty (from AS clause)
	Body       []Statement
	Position   token.Position
}

func (n *ForEachStmt) Pos() token.Position { return n.Position }
func (n *ForEachStmt) stmtNode()           {}

// BreakStmt represents BREAK.
type BreakStmt struct {
	Position token.Position
}

func (n *BreakStmt) Pos() token.Position { return n.Position }
func (n *BreakStmt) stmtNode()           {}

// ContinueStmt represents CONTINUE.
type ContinueStmt struct {
	Position token.Position
}

func (n *ContinueStmt) Pos() token.Position { return n.Position }
func (n *ContinueStmt) stmtNode()           {}

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

// TryStmt represents TRY ... CATCH var ... [FINALLY ...] ENDTRY.
type TryStmt struct {
	Body        []Statement    // try body
	CatchVar    string         // variable name for the caught error
	CatchBody   []Statement    // may be nil
	FinallyBody []Statement    // may be nil
	Position    token.Position
}

func (n *TryStmt) Pos() token.Position { return n.Position }
func (n *TryStmt) stmtNode()           {}

// ---------------------------------------------------------------------------
// Parallel execution
// ---------------------------------------------------------------------------

// ParallelStmt represents PARALLEL [TIMEOUT expr] ... ENDPARALLEL.
type ParallelStmt struct {
	Timeout  Expression // may be nil
	Body     []Statement
	Position token.Position
}

func (n *ParallelStmt) Pos() token.Position { return n.Position }
func (n *ParallelStmt) stmtNode()           {}

// ---------------------------------------------------------------------------
// Device communication
// ---------------------------------------------------------------------------

// ConnectStmt represents CONNECT deviceID protocol address [options...].
type ConnectStmt struct {
	DeviceID string         // device identifier
	Protocol string         // "TCP" or "SERIAL"
	Address  Expression     // address expression
	Options  []Expression   // additional options (e.g. baud rate for serial)
	Position token.Position
}

func (n *ConnectStmt) Pos() token.Position { return n.Position }
func (n *ConnectStmt) stmtNode()           {}

// DisconnectStmt represents DISCONNECT deviceID or DISCONNECT ALL.
type DisconnectStmt struct {
	DeviceID string // device identifier (empty if All is true)
	All      bool   // true for DISCONNECT ALL
	Position token.Position
}

func (n *DisconnectStmt) Pos() token.Position { return n.Position }
func (n *DisconnectStmt) stmtNode()           {}

// SendStmt represents SEND deviceID command.
type SendStmt struct {
	DeviceID string
	Command  Expression
	Position token.Position
}

func (n *SendStmt) Pos() token.Position { return n.Position }
func (n *SendStmt) stmtNode()           {}

// QueryStmt represents QUERY deviceID command resultVar [TIMEOUT expr].
type QueryStmt struct {
	DeviceID  string
	Command   Expression
	ResultVar string
	Timeout   Expression // may be nil
	Position  token.Position
}

func (n *QueryStmt) Pos() token.Position { return n.Position }
func (n *QueryStmt) stmtNode()           {}

// RelayStmt represents RELAY deviceID action channel [state] [resultVar].
type RelayStmt struct {
	DeviceID  string         // device identifier
	Action    string         // "SET", "GET", or "TOGGLE"
	Channel   Expression     // channel expression
	State     Expression     // may be nil (for GET/TOGGLE)
	ResultVar string         // for GET result storage
	Position  token.Position
}

func (n *RelayStmt) Pos() token.Position { return n.Position }
func (n *RelayStmt) stmtNode()           {}

// ---------------------------------------------------------------------------
// Functions
// ---------------------------------------------------------------------------

// FunctionDef represents FUNCTION name(params...) ... ENDFUNCTION.
type FunctionDef struct {
	Name     string
	Params   []string
	Body     []Statement
	Position token.Position
}

func (n *FunctionDef) Pos() token.Position { return n.Position }
func (n *FunctionDef) stmtNode()           {}

// CallExpr represents a function call expression: name(args...).
type CallExpr struct {
	Name     string
	Args     []Expression
	Position token.Position
}

func (n *CallExpr) Pos() token.Position { return n.Position }
func (n *CallExpr) exprNode()           {}

// ReturnStmt represents RETURN [value].
type ReturnStmt struct {
	Value    Expression // may be nil
	Position token.Position
}

func (n *ReturnStmt) Pos() token.Position { return n.Position }
func (n *ReturnStmt) stmtNode()           {}

// ---------------------------------------------------------------------------
// Libraries / imports
// ---------------------------------------------------------------------------

// ImportStmt represents IMPORT "path".
type ImportStmt struct {
	Path     Expression // string literal for the import path
	Position token.Position
}

func (n *ImportStmt) Pos() token.Position { return n.Position }
func (n *ImportStmt) stmtNode()           {}

// LibraryDef represents LIBRARY name ... ENDLIBRARY.
type LibraryDef struct {
	Name     Expression // string expression for the library name
	Body     []Statement
	Position token.Position
}

func (n *LibraryDef) Pos() token.Position { return n.Position }
func (n *LibraryDef) stmtNode()           {}

// ---------------------------------------------------------------------------
// Test results / utilities
// ---------------------------------------------------------------------------

// PassStmt represents PASS message.
type PassStmt struct {
	Message  Expression
	Position token.Position
}

func (n *PassStmt) Pos() token.Position { return n.Position }
func (n *PassStmt) stmtNode()           {}

// FailStmt represents FAIL message.
type FailStmt struct {
	Message  Expression
	Position token.Position
}

func (n *FailStmt) Pos() token.Position { return n.Position }
func (n *FailStmt) stmtNode()           {}

// SkipStmt represents SKIP message.
type SkipStmt struct {
	Message  Expression
	Position token.Position
}

func (n *SkipStmt) Pos() token.Position { return n.Position }
func (n *SkipStmt) stmtNode()           {}

// AssertStmt represents ASSERT condition message.
type AssertStmt struct {
	Condition Expression
	Message   Expression
	Position  token.Position
}

func (n *AssertStmt) Pos() token.Position { return n.Position }
func (n *AssertStmt) stmtNode()           {}

// LogStmt represents LOG level message.
type LogStmt struct {
	Level    string     // e.g. "INFO", "WARN", "ERROR"
	Message  Expression
	Position token.Position
}

func (n *LogStmt) Pos() token.Position { return n.Position }
func (n *LogStmt) stmtNode()           {}

// DelayStmt represents DELAY duration.
type DelayStmt struct {
	Duration Expression
	Position token.Position
}

func (n *DelayStmt) Pos() token.Position { return n.Position }
func (n *DelayStmt) stmtNode()           {}

// ReserveStmt represents RESERVE name size.
type ReserveStmt struct {
	Name     string
	Size     Expression
	Position token.Position
}

func (n *ReserveStmt) Pos() token.Position { return n.Position }
func (n *ReserveStmt) stmtNode()           {}

// ---------------------------------------------------------------------------
// Expressions
// ---------------------------------------------------------------------------

// BinaryExpr represents left op right.
type BinaryExpr struct {
	Left     Expression
	Op       token.TokenType
	Right    Expression
	Position token.Position
}

func (n *BinaryExpr) Pos() token.Position { return n.Position }
func (n *BinaryExpr) exprNode()           {}

// UnaryExpr represents op operand (e.g. !x, -5).
type UnaryExpr struct {
	Op       token.TokenType
	Operand  Expression
	Position token.Position
}

func (n *UnaryExpr) Pos() token.Position { return n.Position }
func (n *UnaryExpr) exprNode()           {}

// NumberLit represents an integer or floating-point literal.
type NumberLit struct {
	Value    string // raw literal value, parsed at runtime
	IsFloat  bool
	Position token.Position
}

func (n *NumberLit) Pos() token.Position { return n.Position }
func (n *NumberLit) exprNode()           {}

// StringLit represents a string literal (escapes already resolved by lexer).
type StringLit struct {
	Value    string
	Position token.Position
}

func (n *StringLit) Pos() token.Position { return n.Position }
func (n *StringLit) exprNode()           {}

// BoolLit represents a boolean literal (TRUE or FALSE).
type BoolLit struct {
	Value    bool
	Position token.Position
}

func (n *BoolLit) Pos() token.Position { return n.Position }
func (n *BoolLit) exprNode()           {}

// NullLit represents the NULL literal.
type NullLit struct {
	Position token.Position
}

func (n *NullLit) Pos() token.Position { return n.Position }
func (n *NullLit) exprNode()           {}

// Identifier represents a variable or name reference.
type Identifier struct {
	Name     string
	Position token.Position
}

func (n *Identifier) Pos() token.Position { return n.Position }
func (n *Identifier) exprNode()           {}

// IndexExpr represents array[index], dict["key"], or obj.field access.
type IndexExpr struct {
	Object   Expression
	Index    Expression
	Position token.Position
}

func (n *IndexExpr) Pos() token.Position { return n.Position }
func (n *IndexExpr) exprNode()           {}

// ArrayLit represents [elem1, elem2, ...].
type ArrayLit struct {
	Elements []Expression
	Position token.Position
}

func (n *ArrayLit) Pos() token.Position { return n.Position }
func (n *ArrayLit) exprNode()           {}

// DictLit represents {"key1": val1, "key2": val2, ...}.
type DictLit struct {
	Keys     []Expression
	Values   []Expression
	Position token.Position
}

func (n *DictLit) Pos() token.Position { return n.Position }
func (n *DictLit) exprNode()           {}

// BuiltinCallExpr represents a builtin function call like LENGTH(arr),
// FLOAT(x), STRING(y), etc.
type BuiltinCallExpr struct {
	Name     string
	Args     []Expression
	Position token.Position
}

func (n *BuiltinCallExpr) Pos() token.Position { return n.Position }
func (n *BuiltinCallExpr) exprNode()           {}
