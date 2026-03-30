package variable

import "testing"

// ---------------------------------------------------------------------------
// Get / Set basics
// ---------------------------------------------------------------------------

func TestGetSetBasic(t *testing.T) {
	env := NewEnvironment()
	if err := env.Set("x", int64(42)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := env.Get("x")
	if !ok {
		t.Fatal("expected x to exist")
	}
	if got != int64(42) {
		t.Errorf("got %v, want 42", got)
	}
}

func TestGetMissing(t *testing.T) {
	env := NewEnvironment()
	_, ok := env.Get("missing")
	if ok {
		t.Error("expected missing variable to not be found")
	}
}

func TestSetOverwrites(t *testing.T) {
	env := NewEnvironment()
	if err := env.Set("x", int64(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := env.Set("x", int64(2)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := env.Get("x")
	if got != int64(2) {
		t.Errorf("got %v, want 2", got)
	}
}

// ---------------------------------------------------------------------------
// Scope chain lookup
// ---------------------------------------------------------------------------

func TestGetWalksScopeChain(t *testing.T) {
	env := NewEnvironment()
	if err := env.Set("global_var", "from_global"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env.PushScope()
	// Should find global_var from child scope
	got, ok := env.Get("global_var")
	if !ok {
		t.Fatal("expected global_var to be visible from child scope")
	}
	if got != "from_global" {
		t.Errorf("got %v, want from_global", got)
	}
	env.PopScope()
}

func TestSetInScopeWhereVariableExists(t *testing.T) {
	env := NewEnvironment()
	if err := env.Set("x", int64(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env.PushScope()
	// Set should update in global scope where x exists, not create in current
	if err := env.Set("x", int64(99)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env.PopScope()
	// After popping, x in global should be 99
	got, _ := env.Get("x")
	if got != int64(99) {
		t.Errorf("got %v, want 99", got)
	}
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestSetConstPreventsModification(t *testing.T) {
	env := NewEnvironment()
	if err := env.SetConst("PI", 3.14159); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify the value
	got, ok := env.Get("PI")
	if !ok {
		t.Fatal("expected PI to exist")
	}
	if got != 3.14159 {
		t.Errorf("got %v, want 3.14159", got)
	}
	// Attempting to Set should fail
	err := env.Set("PI", 3.0)
	if err == nil {
		t.Fatal("expected error when modifying constant")
	}
}

func TestSetConstDuplicate(t *testing.T) {
	env := NewEnvironment()
	if err := env.SetConst("C", int64(299792458)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err := env.SetConst("C", int64(300000000))
	if err == nil {
		t.Fatal("expected error when redefining constant")
	}
}

// ---------------------------------------------------------------------------
// SetGlobal
// ---------------------------------------------------------------------------

func TestSetGlobalFromNestedScope(t *testing.T) {
	env := NewEnvironment()
	env.PushScope()
	env.PushScope()
	if err := env.SetGlobal("g", "global_value"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env.PopScope()
	env.PopScope()
	// After popping back to global, the variable should exist
	got, ok := env.Get("g")
	if !ok {
		t.Fatal("expected g to exist in global scope")
	}
	if got != "global_value" {
		t.Errorf("got %v, want global_value", got)
	}
}

func TestSetGlobalConstantError(t *testing.T) {
	env := NewEnvironment()
	if err := env.SetConst("immutable", int64(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err := env.SetGlobal("immutable", int64(2))
	if err == nil {
		t.Fatal("expected error when setting global constant")
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestDeleteBasic(t *testing.T) {
	env := NewEnvironment()
	if err := env.Set("temp", int64(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := env.Delete("temp"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Exists("temp") {
		t.Error("expected temp to be deleted")
	}
}

func TestDeleteConstantError(t *testing.T) {
	env := NewEnvironment()
	if err := env.SetConst("fixed", int64(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err := env.Delete("fixed")
	if err == nil {
		t.Fatal("expected error when deleting constant")
	}
}

func TestDeleteNotFound(t *testing.T) {
	env := NewEnvironment()
	err := env.Delete("nonexistent")
	if err == nil {
		t.Fatal("expected error when deleting nonexistent variable")
	}
}

// ---------------------------------------------------------------------------
// Exists
// ---------------------------------------------------------------------------

func TestExists(t *testing.T) {
	env := NewEnvironment()
	if env.Exists("x") {
		t.Error("expected x to not exist")
	}
	if err := env.Set("x", int64(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !env.Exists("x") {
		t.Error("expected x to exist")
	}
}

// ---------------------------------------------------------------------------
// PushScope / PopScope
// ---------------------------------------------------------------------------

func TestPushPopScope(t *testing.T) {
	env := NewEnvironment()
	if err := env.Set("outer", int64(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env.PushScope()
	if err := env.Set("inner", int64(2)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both should be visible
	if !env.Exists("outer") {
		t.Error("expected outer to be visible from inner scope")
	}
	if !env.Exists("inner") {
		t.Error("expected inner to be visible in current scope")
	}
	env.PopScope()
	// After pop, inner should no longer be visible
	if env.Exists("inner") {
		t.Error("expected inner to not be visible after pop")
	}
	if !env.Exists("outer") {
		t.Error("expected outer to still exist after pop")
	}
}

func TestPopScopeOnGlobalIsNoOp(t *testing.T) {
	env := NewEnvironment()
	if err := env.Set("x", int64(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Pop on global scope should be a no-op
	env.PopScope()
	got, ok := env.Get("x")
	if !ok || got != int64(1) {
		t.Error("expected global scope to remain intact after PopScope on global")
	}
}

// ---------------------------------------------------------------------------
// PushFunctionScope
// ---------------------------------------------------------------------------

func TestPushFunctionScopeIsolatesFromNonGlobal(t *testing.T) {
	env := NewEnvironment()
	if err := env.Set("global_var", int64(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env.PushScope()
	if err := env.Set("local_var", int64(2)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env.PushFunctionScope()
	// Should NOT see local_var (it's in the non-global parent scope)
	if env.Exists("local_var") {
		t.Error("expected local_var to NOT be visible in function scope")
	}
	// Should see global_var (function scope parents to global)
	if !env.Exists("global_var") {
		t.Error("expected global_var to be visible in function scope")
	}
	env.PopFunctionScope()
}

func TestPushFunctionScopeCanSeeGlobals(t *testing.T) {
	env := NewEnvironment()
	if err := env.Set("a", int64(10)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := env.SetConst("B", int64(20)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env.PushFunctionScope()
	gotA, okA := env.Get("a")
	gotB, okB := env.Get("B")
	if !okA || gotA != int64(10) {
		t.Errorf("expected a=10 in function scope, got %v (found=%v)", gotA, okA)
	}
	if !okB || gotB != int64(20) {
		t.Errorf("expected B=20 in function scope, got %v (found=%v)", gotB, okB)
	}
	env.PopFunctionScope()
}

// ---------------------------------------------------------------------------
// Multiple nested scopes
// ---------------------------------------------------------------------------

func TestMultipleNestedScopes(t *testing.T) {
	env := NewEnvironment()
	if err := env.Set("a", int64(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env.PushScope()
	if err := env.Set("b", int64(2)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env.PushScope()
	if err := env.Set("c", int64(3)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env.PushScope()
	if err := env.Set("d", int64(4)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All should be visible from innermost scope
	for _, name := range []string{"a", "b", "c", "d"} {
		if !env.Exists(name) {
			t.Errorf("expected %s to exist in innermost scope", name)
		}
	}

	env.PopScope() // pop d scope
	if env.Exists("d") {
		t.Error("expected d to not be visible after pop")
	}
	if !env.Exists("c") {
		t.Error("expected c to still be visible")
	}

	env.PopScope() // pop c scope
	env.PopScope() // pop b scope

	if env.Exists("b") {
		t.Error("expected b to not be visible after pop")
	}
	if !env.Exists("a") {
		t.Error("expected a to still be visible in global")
	}
}

// ---------------------------------------------------------------------------
// Shadowing
// ---------------------------------------------------------------------------

func TestScopeShadowing(t *testing.T) {
	env := NewEnvironment()
	if err := env.Set("x", int64(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env.PushScope()
	// Set x in child scope — since x already exists in parent, this updates
	// the parent value per the Set semantics
	if err := env.Set("x", int64(2)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := env.Get("x")
	if got != int64(2) {
		t.Errorf("got %v, want 2", got)
	}
	env.PopScope()
	// The global x should now be 2 (was updated in place)
	got, _ = env.Get("x")
	if got != int64(2) {
		t.Errorf("after pop: got %v, want 2", got)
	}
}
