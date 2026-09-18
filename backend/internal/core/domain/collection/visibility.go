package collection

import (
	"fmt"
	"reflect"
	"strings"
)

// CondOp is a leaf comparison operator in a visibility Condition tree.
type CondOp string

const (
	CondOpEq       CondOp = "eq"
	CondOpNeq      CondOp = "neq"
	CondOpGt       CondOp = "gt"
	CondOpGte      CondOp = "gte"
	CondOpLt       CondOp = "lt"
	CondOpLte      CondOp = "lte"
	CondOpIn       CondOp = "in"
	CondOpNin      CondOp = "nin"
	CondOpEmpty    CondOp = "empty"
	CondOpNempty   CondOp = "nempty"
	CondOpContains CondOp = "contains"
)

// Condition is a typed visibility tree: either a group (and/or) or a leaf
// (field + op + value). Groups and leaves are mutually exclusive.
type Condition struct {
	And []Condition `json:"and,omitempty"`
	Or  []Condition `json:"or,omitempty"`

	Field string `json:"field,omitempty"`
	Op    CondOp `json:"op,omitempty"`
	Value any    `json:"value,omitempty"`
}

// Validate checks the tree shape and that every leaf op is known.
// knownFields, when non-nil, rejects leaf field names not in the set.
func (c *Condition) Validate(knownFields map[string]bool) error {
	if c == nil {
		return nil
	}
	hasAnd := len(c.And) > 0
	hasOr := len(c.Or) > 0
	hasLeaf := c.Field != "" || c.Op != ""

	n := 0
	if hasAnd {
		n++
	}
	if hasOr {
		n++
	}
	if hasLeaf {
		n++
	}
	if n == 0 {
		return fmt.Errorf("condition must be an and/or group or a leaf")
	}
	if n > 1 {
		return fmt.Errorf("condition cannot mix and/or groups with a leaf")
	}

	if hasAnd {
		for i := range c.And {
			if err := c.And[i].Validate(knownFields); err != nil {
				return err
			}
		}
		return nil
	}
	if hasOr {
		for i := range c.Or {
			if err := c.Or[i].Validate(knownFields); err != nil {
				return err
			}
		}
		return nil
	}

	if c.Field == "" {
		return fmt.Errorf("leaf condition requires field")
	}
	if knownFields != nil && !knownFields[c.Field] {
		return fmt.Errorf("unknown field %q in visibility condition", c.Field)
	}
	switch c.Op {
	case CondOpEq, CondOpNeq, CondOpGt, CondOpGte, CondOpLt, CondOpLte,
		CondOpIn, CondOpNin, CondOpEmpty, CondOpNempty, CondOpContains:
		return nil
	case "":
		return fmt.Errorf("leaf condition requires op")
	default:
		return fmt.Errorf("unknown condition op %q", c.Op)
	}
}

// Evaluate returns whether the condition holds against the given values map
// (keyed by field name). A nil condition is always true. Missing fields are
// treated as empty.
func (c *Condition) Evaluate(values map[string]any) (bool, error) {
	if c == nil {
		return true, nil
	}
	if err := c.Validate(nil); err != nil {
		return false, err
	}
	return c.eval(values)
}

func (c *Condition) eval(values map[string]any) (bool, error) {
	if len(c.And) > 0 {
		for i := range c.And {
			ok, err := c.And[i].eval(values)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
		return true, nil
	}
	if len(c.Or) > 0 {
		for i := range c.Or {
			ok, err := c.Or[i].eval(values)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	return evalLeaf(c.Field, c.Op, c.Value, values)
}

func evalLeaf(field string, op CondOp, expected any, values map[string]any) (bool, error) {
	var actual any
	if values != nil {
		actual = values[field]
	}
	empty := isEmpty(actual)

	switch op {
	case CondOpEmpty:
		return empty, nil
	case CondOpNempty:
		return !empty, nil
	case CondOpEq:
		return equalValues(actual, expected), nil
	case CondOpNeq:
		return !equalValues(actual, expected), nil
	case CondOpIn:
		return containsValue(expected, actual), nil
	case CondOpNin:
		return !containsValue(expected, actual), nil
	case CondOpContains:
		return stringContains(actual, expected), nil
	case CondOpGt, CondOpGte, CondOpLt, CondOpLte:
		cmp, ok := compareOrdered(actual, expected)
		if !ok {
			return false, nil
		}
		switch op {
		case CondOpGt:
			return cmp > 0, nil
		case CondOpGte:
			return cmp >= 0, nil
		case CondOpLt:
			return cmp < 0, nil
		case CondOpLte:
			return cmp <= 0, nil
		}
	}
	return false, fmt.Errorf("unsupported op %q", op)
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

func equalValues(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	af, aOK := toFloat(a)
	bf, bOK := toFloat(b)
	if aOK && bOK {
		return af == bf
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func containsValue(list any, item any) bool {
	rv := reflect.ValueOf(list)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return false
	}
	for i := 0; i < rv.Len(); i++ {
		if equalValues(rv.Index(i).Interface(), item) {
			return true
		}
	}
	return false
}

func stringContains(actual, expected any) bool {
	return strings.Contains(fmt.Sprint(actual), fmt.Sprint(expected))
}

func compareOrdered(a, b any) (int, bool) {
	af, aOK := toFloat(a)
	bf, bOK := toFloat(b)
	if aOK && bOK {
		switch {
		case af < bf:
			return -1, true
		case af > bf:
			return 1, true
		default:
			return 0, true
		}
	}
	as, bs := fmt.Sprint(a), fmt.Sprint(b)
	return strings.Compare(as, bs), true
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case int:
		return float64(t), true
	case int32:
		return float64(t), true
	case int64:
		return float64(t), true
	case float32:
		return float64(t), true
	case float64:
		return t, true
	}
	type float64er interface {
		Float64() (float64, error)
	}
	if n, ok := v.(float64er); ok {
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}
