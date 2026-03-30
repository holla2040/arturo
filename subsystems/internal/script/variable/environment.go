package variable

import "fmt"

// scope is a single variable scope with an optional parent pointer.
type scope struct {
	vars      map[string]interface{}
	constants map[string]bool // tracks which names are constants
	parent    *scope
}

// newScope creates a new empty scope with the given parent.
func newScope(parent *scope) *scope {
	return &scope{
		vars:      make(map[string]interface{}),
		constants: make(map[string]bool),
		parent:    parent,
	}
}

// Environment provides scoped variable storage for the script engine.
type Environment struct {
	global    *scope
	current   *scope
	savedScopes []*scope // stack of saved scopes for function calls
}

// NewEnvironment creates a new Environment with an empty global scope.
func NewEnvironment() *Environment {
	g := newScope(nil)
	return &Environment{
		global:  g,
		current: g,
	}
}

// ---------------------------------------------------------------------------
// Variable operations
// ---------------------------------------------------------------------------

// Get retrieves a variable by walking up the scope chain from current to global.
// Returns the value and true if found, or (nil, false) if not found.
func (e *Environment) Get(name string) (interface{}, bool) {
	for s := e.current; s != nil; s = s.parent {
		if val, ok := s.vars[name]; ok {
			return val, true
		}
	}
	return nil, false
}

// Set assigns a value to a variable. If the variable already exists in an
// ancestor scope, it is updated there. Otherwise it is created in the current
// scope. Returns an error if the variable is a constant.
func (e *Environment) Set(name string, value interface{}) error {
	// Walk up to find existing variable
	for s := e.current; s != nil; s = s.parent {
		if _, ok := s.vars[name]; ok {
			if s.constants[name] {
				return fmt.Errorf("cannot assign to constant %q", name)
			}
			s.vars[name] = value
			return nil
		}
	}
	// New variable: create in current scope
	e.current.vars[name] = value
	return nil
}

// SetConst declares a constant in the current scope. Returns an error if the
// name already exists as a constant in the current scope.
func (e *Environment) SetConst(name string, value interface{}) error {
	if e.current.constants[name] {
		return fmt.Errorf("constant %q already defined", name)
	}
	e.current.vars[name] = value
	e.current.constants[name] = true
	return nil
}

// SetGlobal sets a variable directly in the global scope. Returns an error if
// the variable is a constant in the global scope.
func (e *Environment) SetGlobal(name string, value interface{}) error {
	if e.global.constants[name] {
		return fmt.Errorf("cannot assign to constant %q", name)
	}
	e.global.vars[name] = value
	return nil
}

// Delete removes a variable from the scope where it exists. Returns an error
// if the variable is a constant or does not exist.
func (e *Environment) Delete(name string) error {
	for s := e.current; s != nil; s = s.parent {
		if _, ok := s.vars[name]; ok {
			if s.constants[name] {
				return fmt.Errorf("cannot delete constant %q", name)
			}
			delete(s.vars, name)
			return nil
		}
	}
	return fmt.Errorf("variable %q not found", name)
}

// Exists returns true if the variable exists in any scope from current to global.
func (e *Environment) Exists(name string) bool {
	_, ok := e.Get(name)
	return ok
}

// ---------------------------------------------------------------------------
// Scope management
// ---------------------------------------------------------------------------

// PushScope creates a new child scope with the current scope as parent and
// makes it the current scope.
func (e *Environment) PushScope() {
	e.current = newScope(e.current)
}

// PopScope returns to the parent scope. If already at the global scope, this
// is a no-op.
func (e *Environment) PopScope() {
	if e.current.parent != nil {
		e.current = e.current.parent
	}
}

// PushFunctionScope creates a new scope whose parent is the global scope,
// isolating it from any intermediate scopes. The previous current scope is
// saved so that PopFunctionScope can restore it, enabling recursive calls.
func (e *Environment) PushFunctionScope() {
	e.savedScopes = append(e.savedScopes, e.current)
	e.current = newScope(e.global)
}

// PopFunctionScope restores the scope that was active before the matching
// PushFunctionScope call. If no saved scope exists, it falls back to PopScope.
func (e *Environment) PopFunctionScope() {
	n := len(e.savedScopes)
	if n > 0 {
		e.current = e.savedScopes[n-1]
		e.savedScopes = e.savedScopes[:n-1]
	} else {
		e.PopScope()
	}
}

// SetLocal assigns a value to a variable in the current scope only, without
// walking up the scope chain. This is used for binding function parameters
// so they shadow any identically-named variables in parent scopes.
func (e *Environment) SetLocal(name string, value interface{}) error {
	if e.current.constants[name] {
		return fmt.Errorf("cannot assign to constant %q", name)
	}
	e.current.vars[name] = value
	return nil
}
