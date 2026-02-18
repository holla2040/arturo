package variable

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Type helpers
// ---------------------------------------------------------------------------

// ToFloat converts int64, float64, or string to float64.
func ToFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case int64:
		return float64(val), nil
	case float64:
		return val, nil
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot convert string %q to float: %w", val, err)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("cannot convert %s to float", TypeName(v))
	}
}

// ToInt converts int64, float64, or string to int64.
func ToInt(v interface{}) (int64, error) {
	switch val := v.(type) {
	case int64:
		return val, nil
	case float64:
		return int64(val), nil
	case string:
		i, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot convert string %q to int: %w", val, err)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("cannot convert %s to int", TypeName(v))
	}
}

// ToString converts any value to its string representation. Always succeeds.
func ToString(v interface{}) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case string:
		return val
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// ToBool returns the truthiness of a value.
// Truthy: non-zero numbers, non-empty strings, true, non-nil, non-empty arrays/maps.
func ToBool(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case int64:
		return val != 0
	case float64:
		return val != 0
	case string:
		return val != ""
	case []interface{}:
		return len(val) > 0
	case map[string]interface{}:
		return len(val) > 0
	default:
		return true
	}
}

// IsTruthy is an alias for ToBool.
func IsTruthy(v interface{}) bool {
	return ToBool(v)
}

// TypeName returns the type name of a value as used by the script engine.
func TypeName(v interface{}) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case int64:
		return "int"
	case float64:
		return "float"
	case string:
		return "string"
	case bool:
		return "bool"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "dict"
	default:
		return "unknown"
	}
}

// ---------------------------------------------------------------------------
// Arithmetic operations
// ---------------------------------------------------------------------------

// Add performs addition. int+int=int, float+any_num=float, string+any=string concat.
func Add(a, b interface{}) (interface{}, error) {
	// String concatenation: string + anything = string
	if sa, ok := a.(string); ok {
		return sa + ToString(b), nil
	}

	fa, aIsFloat := toNumeric(a)
	fb, bIsFloat := toNumeric(b)

	if !isNumericType(a) {
		return nil, fmt.Errorf("cannot add %s and %s", TypeName(a), TypeName(b))
	}
	if !isNumericType(b) {
		return nil, fmt.Errorf("cannot add %s and %s", TypeName(a), TypeName(b))
	}

	// If either is float, result is float
	if aIsFloat || bIsFloat {
		return fa + fb, nil
	}

	// Both are int64
	ai, _ := a.(int64)
	bi, _ := b.(int64)
	return ai + bi, nil
}

// Subtract performs subtraction. Numeric only.
func Subtract(a, b interface{}) (interface{}, error) {
	if !isNumericType(a) || !isNumericType(b) {
		return nil, fmt.Errorf("cannot subtract %s and %s", TypeName(a), TypeName(b))
	}

	fa, aIsFloat := toNumeric(a)
	fb, bIsFloat := toNumeric(b)

	if aIsFloat || bIsFloat {
		return fa - fb, nil
	}

	ai, _ := a.(int64)
	bi, _ := b.(int64)
	return ai - bi, nil
}

// Multiply performs multiplication. Numeric only.
func Multiply(a, b interface{}) (interface{}, error) {
	if !isNumericType(a) || !isNumericType(b) {
		return nil, fmt.Errorf("cannot multiply %s and %s", TypeName(a), TypeName(b))
	}

	fa, aIsFloat := toNumeric(a)
	fb, bIsFloat := toNumeric(b)

	if aIsFloat || bIsFloat {
		return fa * fb, nil
	}

	ai, _ := a.(int64)
	bi, _ := b.(int64)
	return ai * bi, nil
}

// Divide performs division. Returns error on division by zero.
// int/int returns int (truncated), float/any_num returns float.
func Divide(a, b interface{}) (interface{}, error) {
	if !isNumericType(a) || !isNumericType(b) {
		return nil, fmt.Errorf("cannot divide %s by %s", TypeName(a), TypeName(b))
	}

	fa, aIsFloat := toNumeric(a)
	fb, bIsFloat := toNumeric(b)

	if fb == 0 {
		return nil, fmt.Errorf("division by zero")
	}

	if aIsFloat || bIsFloat {
		return fa / fb, nil
	}

	ai, _ := a.(int64)
	bi, _ := b.(int64)
	return ai / bi, nil
}

// Modulo performs the modulo operation. int64 only.
func Modulo(a, b interface{}) (interface{}, error) {
	ai, aOk := a.(int64)
	bi, bOk := b.(int64)

	if !aOk || !bOk {
		return nil, fmt.Errorf("modulo requires int operands, got %s and %s", TypeName(a), TypeName(b))
	}

	if bi == 0 {
		return nil, fmt.Errorf("division by zero")
	}

	return ai % bi, nil
}

// Compare returns -1, 0, or 1 comparing a and b. Works for numbers and strings.
func Compare(a, b interface{}) (int, error) {
	// Both strings
	sa, aStr := a.(string)
	sb, bStr := b.(string)
	if aStr && bStr {
		return strings.Compare(sa, sb), nil
	}

	// Both numeric
	if isNumericType(a) && isNumericType(b) {
		fa, _ := toNumeric(a)
		fb, _ := toNumeric(b)
		switch {
		case fa < fb:
			return -1, nil
		case fa > fb:
			return 1, nil
		default:
			return 0, nil
		}
	}

	return 0, fmt.Errorf("cannot compare %s and %s", TypeName(a), TypeName(b))
}

// Equal performs deep equality comparison.
func Equal(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Numeric comparison: allow int64 == float64 if values match
	if isNumericType(a) && isNumericType(b) {
		fa, _ := toNumeric(a)
		fb, _ := toNumeric(b)
		return fa == fb
	}

	return reflect.DeepEqual(a, b)
}

// Negate performs unary minus on a numeric value.
func Negate(v interface{}) (interface{}, error) {
	switch val := v.(type) {
	case int64:
		return -val, nil
	case float64:
		return -val, nil
	default:
		return nil, fmt.Errorf("cannot negate %s", TypeName(v))
	}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// isNumericType returns true if v is int64 or float64.
func isNumericType(v interface{}) bool {
	switch v.(type) {
	case int64, float64:
		return true
	default:
		return false
	}
}

// toNumeric converts int64 or float64 to float64 and reports whether the
// original value was float64.
func toNumeric(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case int64:
		return float64(val), false
	case float64:
		return val, true
	default:
		return math.NaN(), false
	}
}
