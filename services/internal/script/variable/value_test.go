package variable

import (
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// ToFloat
// ---------------------------------------------------------------------------

func TestToFloat(t *testing.T) {
	cases := []struct {
		name    string
		input   interface{}
		want    float64
		wantErr bool
	}{
		{"from int64", int64(42), 42.0, false},
		{"from float64", 3.14, 3.14, false},
		{"from string", "2.5", 2.5, false},
		{"from invalid string", "abc", 0, true},
		{"from bool", true, 0, true},
		{"from nil", nil, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToFloat(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ToInt
// ---------------------------------------------------------------------------

func TestToInt(t *testing.T) {
	cases := []struct {
		name    string
		input   interface{}
		want    int64
		wantErr bool
	}{
		{"from int64", int64(42), 42, false},
		{"from float64", 3.9, 3, false},
		{"from string", "100", 100, false},
		{"from invalid string", "nope", 0, true},
		{"from bool", true, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToInt(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ToString
// ---------------------------------------------------------------------------

func TestToString(t *testing.T) {
	cases := []struct {
		name  string
		input interface{}
		want  string
	}{
		{"string", "hello", "hello"},
		{"int64", int64(42), "42"},
		{"float64", 3.14, "3.14"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"nil", nil, "null"},
		{"array", []interface{}{int64(1), int64(2)}, "[1 2]"},
		{"map", map[string]interface{}{"a": int64(1)}, "map[a:1]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToString(tc.input)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ToBool / IsTruthy
// ---------------------------------------------------------------------------

func TestToBool(t *testing.T) {
	cases := []struct {
		name  string
		input interface{}
		want  bool
	}{
		{"true", true, true},
		{"false", false, false},
		{"zero int", int64(0), false},
		{"nonzero int", int64(1), true},
		{"negative int", int64(-5), true},
		{"zero float", 0.0, false},
		{"nonzero float", 0.1, true},
		{"empty string", "", false},
		{"non-empty string", "hi", true},
		{"nil", nil, false},
		{"empty array", []interface{}{}, false},
		{"non-empty array", []interface{}{int64(1)}, true},
		{"empty map", map[string]interface{}{}, false},
		{"non-empty map", map[string]interface{}{"k": "v"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToBool(tc.input)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsTruthyAlias(t *testing.T) {
	// IsTruthy is an alias for ToBool; just verify it works the same.
	if IsTruthy(int64(0)) != false {
		t.Error("IsTruthy(0) should be false")
	}
	if IsTruthy(int64(1)) != true {
		t.Error("IsTruthy(1) should be true")
	}
}

// ---------------------------------------------------------------------------
// TypeName
// ---------------------------------------------------------------------------

func TestTypeName(t *testing.T) {
	cases := []struct {
		name  string
		input interface{}
		want  string
	}{
		{"int64", int64(1), "int"},
		{"float64", 1.0, "float"},
		{"string", "hi", "string"},
		{"bool", true, "bool"},
		{"nil", nil, "null"},
		{"array", []interface{}{}, "array"},
		{"dict", map[string]interface{}{}, "dict"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TypeName(tc.input)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Add
// ---------------------------------------------------------------------------

func TestAdd(t *testing.T) {
	cases := []struct {
		name    string
		a, b    interface{}
		want    interface{}
		wantErr bool
	}{
		{"int+int", int64(2), int64(3), int64(5), false},
		{"float+int", 2.5, int64(3), 5.5, false},
		{"int+float", int64(3), 2.5, 5.5, false},
		{"float+float", 1.5, 2.5, 4.0, false},
		{"string+string", "hello ", "world", "hello world", false},
		{"string+int", "val=", int64(42), "val=42", false},
		{"string+nil", "x=", nil, "x=null", false},
		{"bool+int error", true, int64(1), nil, true},
		{"nil+int error", nil, int64(1), nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Add(tc.a, tc.b)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !Equal(got, tc.want) {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Subtract
// ---------------------------------------------------------------------------

func TestSubtract(t *testing.T) {
	cases := []struct {
		name    string
		a, b    interface{}
		want    interface{}
		wantErr bool
	}{
		{"int-int", int64(10), int64(3), int64(7), false},
		{"float-int", 10.5, int64(3), 7.5, false},
		{"int-float", int64(10), 3.5, 6.5, false},
		{"string error", "a", int64(1), nil, true},
		{"bool error", true, false, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Subtract(tc.a, tc.b)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !Equal(got, tc.want) {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Multiply
// ---------------------------------------------------------------------------

func TestMultiply(t *testing.T) {
	cases := []struct {
		name    string
		a, b    interface{}
		want    interface{}
		wantErr bool
	}{
		{"int*int", int64(4), int64(5), int64(20), false},
		{"float*float", 2.0, 3.0, 6.0, false},
		{"float*int", 2.5, int64(4), 10.0, false},
		{"string error", "a", int64(2), nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Multiply(tc.a, tc.b)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !Equal(got, tc.want) {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Divide
// ---------------------------------------------------------------------------

func TestDivide(t *testing.T) {
	cases := []struct {
		name    string
		a, b    interface{}
		want    interface{}
		wantErr bool
	}{
		{"int/int truncated", int64(7), int64(2), int64(3), false},
		{"int/int exact", int64(10), int64(5), int64(2), false},
		{"float/int", 7.0, int64(2), 3.5, false},
		{"int/float", int64(7), 2.0, 3.5, false},
		{"divide by zero int", int64(1), int64(0), nil, true},
		{"divide by zero float", 1.0, 0.0, nil, true},
		{"string error", "a", int64(1), nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Divide(tc.a, tc.b)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !Equal(got, tc.want) {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Modulo
// ---------------------------------------------------------------------------

func TestModulo(t *testing.T) {
	cases := []struct {
		name    string
		a, b    interface{}
		want    interface{}
		wantErr bool
	}{
		{"int%int", int64(10), int64(3), int64(1), false},
		{"int%int exact", int64(9), int64(3), int64(0), false},
		{"float%int error", 10.0, int64(3), nil, true},
		{"int%float error", int64(10), 3.0, nil, true},
		{"divide by zero", int64(10), int64(0), nil, true},
		{"string error", "a", int64(1), nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Modulo(tc.a, tc.b)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !Equal(got, tc.want) {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Compare
// ---------------------------------------------------------------------------

func TestCompare(t *testing.T) {
	cases := []struct {
		name    string
		a, b    interface{}
		want    int
		wantErr bool
	}{
		{"int less", int64(1), int64(2), -1, false},
		{"int equal", int64(5), int64(5), 0, false},
		{"int greater", int64(10), int64(3), 1, false},
		{"float less", 1.0, 2.0, -1, false},
		{"int vs float", int64(3), 3.0, 0, false},
		{"string less", "abc", "def", -1, false},
		{"string equal", "abc", "abc", 0, false},
		{"string greater", "def", "abc", 1, false},
		{"incompatible", "abc", int64(1), 0, true},
		{"bool error", true, false, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Compare(tc.a, tc.b)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Equal
// ---------------------------------------------------------------------------

func TestEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b interface{}
		want bool
	}{
		{"same int", int64(5), int64(5), true},
		{"different int", int64(5), int64(6), false},
		{"same float", 3.14, 3.14, true},
		{"int and float same value", int64(5), 5.0, true},
		{"int and float different", int64(5), 5.1, false},
		{"same string", "abc", "abc", true},
		{"different string", "abc", "def", false},
		{"same bool", true, true, true},
		{"different bool", true, false, false},
		{"nil nil", nil, nil, true},
		{"nil vs int", nil, int64(0), false},
		{"int vs nil", int64(0), nil, false},
		{"same array", []interface{}{int64(1), int64(2)}, []interface{}{int64(1), int64(2)}, true},
		{"different array", []interface{}{int64(1)}, []interface{}{int64(2)}, false},
		{"same map", map[string]interface{}{"a": int64(1)}, map[string]interface{}{"a": int64(1)}, true},
		{"different map", map[string]interface{}{"a": int64(1)}, map[string]interface{}{"a": int64(2)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Equal(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Negate
// ---------------------------------------------------------------------------

func TestNegate(t *testing.T) {
	cases := []struct {
		name    string
		input   interface{}
		want    interface{}
		wantErr bool
	}{
		{"negate int", int64(5), int64(-5), false},
		{"negate negative int", int64(-3), int64(3), false},
		{"negate zero int", int64(0), int64(0), false},
		{"negate float", 3.14, -3.14, false},
		{"negate string error", "abc", nil, true},
		{"negate bool error", true, nil, true},
		{"negate nil error", nil, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Negate(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !Equal(got, tc.want) {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestToFloatNaN(t *testing.T) {
	// Verify NaN string parses to NaN
	f, err := ToFloat("NaN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !math.IsNaN(f) {
		t.Errorf("expected NaN, got %v", f)
	}
}

func TestToStringFloat(t *testing.T) {
	// Verify float formatting doesn't add trailing zeros
	got := ToString(1.0)
	if got != "1" {
		t.Errorf("got %q, want %q", got, "1")
	}
}
