package executor

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/holla2040/arturo/internal/script/variable"
)

// evalBuiltin evaluates a built-in function call with the given evaluated
// arguments.
func (e *Executor) evalBuiltin(name string, args []interface{}) (interface{}, error) {
	switch strings.ToUpper(name) {
	case "FLOAT":
		if len(args) != 1 {
			return nil, fmt.Errorf("FLOAT() requires 1 argument, got %d", len(args))
		}
		return variable.ToFloat(args[0])

	case "INT":
		if len(args) != 1 {
			return nil, fmt.Errorf("INT() requires 1 argument, got %d", len(args))
		}
		return variable.ToInt(args[0])

	case "STRING":
		if len(args) != 1 {
			return nil, fmt.Errorf("STRING() requires 1 argument, got %d", len(args))
		}
		return variable.ToString(args[0]), nil

	case "BOOL":
		if len(args) != 1 {
			return nil, fmt.Errorf("BOOL() requires 1 argument, got %d", len(args))
		}
		return variable.ToBool(args[0]), nil

	case "LENGTH":
		if len(args) != 1 {
			return nil, fmt.Errorf("LENGTH() requires 1 argument, got %d", len(args))
		}
		return lengthOf(args[0])

	case "TYPE":
		if len(args) != 1 {
			return nil, fmt.Errorf("TYPE() requires 1 argument, got %d", len(args))
		}
		return variable.TypeName(args[0]), nil

	case "EXISTS":
		// EXISTS is handled specially in evalExpression; if we get here, the
		// argument was already evaluated successfully.
		return true, nil

	case "NOW":
		if len(args) != 0 {
			return nil, fmt.Errorf("NOW() takes no arguments, got %d", len(args))
		}
		// Milliseconds since Unix epoch — int64. Scripts do elapsed math as
		// NOW() - start. String concatenation still works via Add's coercion.
		// See docs/reference/SCRIPTING_LANGUAGE_ORIGINAL.md "Built-in Functions".
		return time.Now().UnixMilli(), nil

	case "JSON_GET":
		if len(args) != 2 {
			return nil, fmt.Errorf("JSON_GET() requires 2 arguments (json, field), got %d", len(args))
		}
		jsonStr := variable.ToString(args[0])
		field := variable.ToString(args[1])
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
			return nil, fmt.Errorf("JSON_GET: invalid JSON: %w", err)
		}
		v, ok := m[field]
		if !ok {
			return nil, fmt.Errorf("JSON_GET: field %q not found", field)
		}
		return v, nil

	default:
		return nil, fmt.Errorf("unknown builtin function %q", name)
	}
}

// lengthOf returns the length of a string, array, or map.
func lengthOf(v interface{}) (int64, error) {
	switch val := v.(type) {
	case string:
		return int64(len(val)), nil
	case []interface{}:
		return int64(len(val)), nil
	case map[string]interface{}:
		return int64(len(val)), nil
	default:
		return 0, fmt.Errorf("LENGTH: cannot get length of %s", variable.TypeName(v))
	}
}
