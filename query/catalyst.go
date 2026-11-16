package query

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
)

func (c Catalyst) op() Operator {
	if c.Operator == "" {
		return Eq
	}
	return c.Operator
}
func (c Catalyst) rank() bool { return c.op() == VectorDistance || c.op() == FTScore }
func (c Catalyst) unary() bool {
	switch c.op() {
	case Defined, NotDefined, IsNull, IsString:
		return true
	}
	return false
}
func nilValue(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}
func (c Catalyst) validate() error {
	op := c.op()
	if _, ok := operators[op]; !ok {
		return fmt.Errorf("unrecognized operator %q", op)
	}
	if e := ValidateIdentifier(c.Column); e != nil {
		return e
	}
	if c.Alias != "" {
		if e := validateAlias(c.Alias); e != nil {
			return e
		}
	}
	if _, e := where(c.Where); e != nil {
		return e
	}
	strOp := op == StartsWith || op == StringContains || op == StringEquals
	if c.IgnoreCase && !strOp {
		return fmt.Errorf("IgnoreCase requires a string predicate")
	}
	if c.PartialMatch && op != In && op != NotIn {
		return fmt.Errorf("PartialMatch requires In or NotIn")
	}
	if c.Left != nil || c.Right != nil {
		if c.Left == nil || c.Right == nil || c.Left.Kind == ParameterOperand {
			return fmt.Errorf("comparison requires a field or array length on the left")
		}
		switch op {
		case Eq, NotEq, Gt, Gte, Lt, Lte:
		default:
			return fmt.Errorf("operands support only equality and ordering comparisons")
		}
		for _, operand := range []*Operand{c.Left, c.Right} {
			switch operand.Kind {
			case FieldOperand, ArrayLengthOperand:
				if _, e := formatProperty(operand.Alias, operand.Path, nil); e != nil {
					return e
				}
			case ParameterOperand:
				if nilValue(operand.Value) {
					return fmt.Errorf("null comparison parameter; use IsNull")
				}
			default:
				return fmt.Errorf("invalid operand kind")
			}
		}
		return nil
	}
	if c.unary() {
		if !nilValue(c.Value) {
			return fmt.Errorf("unary predicates cannot bind a value")
		}
		return nil
	}
	if nilValue(c.Value) {
		return fmt.Errorf("operator %s requires a non-null value", op)
	}
	if strOp || op == FTContains || op == NotFTContains {
		if _, ok := c.Value.(string); !ok {
			return fmt.Errorf("operator %s requires a string", op)
		}
	}
	if op == Like || op == NotLike {
		if !strings.Contains(fmt.Sprint(c.Value), "%") {
			return fmt.Errorf("LIKE requires a percent wildcard")
		}
	}
	if op == Contains || op == NotContains {
		kind := reflect.TypeOf(c.Value).Kind()
		if kind != reflect.Array && kind != reflect.Slice {
			return fmt.Errorf("Contains requires a collection")
		}
	}
	if op == VectorDistance {
		r := reflect.ValueOf(c.Value)
		if r.Kind() != reflect.Slice && r.Kind() != reflect.Array {
			return fmt.Errorf("vector must be a float array")
		}
		if r.Len() == 0 || r.Len() > MaxVectorDimensions {
			return fmt.Errorf("invalid vector dimensions")
		}
		for i := 0; i < r.Len(); i++ {
			v := r.Index(i)
			if v.Kind() != reflect.Float32 && v.Kind() != reflect.Float64 {
				return fmt.Errorf("vector must contain floats")
			}
			if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) {
				return fmt.Errorf("vector must contain finite values")
			}
		}
	}
	switch op {
	case FTContainsAll, NotFTContainsAll, FTContainsAny, NotFTContainsAny, FTScore:
		values, ok := c.Value.([]string)
		if !ok || len(values) == 0 {
			return fmt.Errorf("full-text operation requires non-empty string array")
		}
		for _, v := range values {
			if strings.TrimSpace(v) == "" {
				return fmt.Errorf("full-text terms cannot be blank")
			}
		}
	}
	return nil
}
func where(w Where) (string, error) {
	if w == "" || w == And {
		return "AND", nil
	}
	if w == Or {
		return "OR", nil
	}
	return "", fmt.Errorf("unrecognized WHERE %q", w)
}
func parameterName(c Catalyst, index int) string {
	id := c.ID
	if id == "" {
		id = fmt.Sprint(index)
	}
	b, _ := json.Marshal([]string{c.Column, id})
	return fmt.Sprintf("p_%x", sha256.Sum256(b))
}
func (b *builder) operand(o *Operand, name string) (string, error) {
	if o.Kind == ParameterOperand {
		return "@" + name, nil
	}
	p, e := b.property(o.Alias, o.Path)
	if o.Kind == ArrayLengthOperand {
		p = "ARRAY_LENGTH(" + p + ")"
	}
	return p, e
}
func (b *builder) predicate(c Catalyst, name string) (string, error) {
	if c.Left != nil {
		left, e := b.operand(c.Left, name)
		if e != nil {
			return "", e
		}
		right, e := b.operand(c.Right, name)
		return left + " " + operators[c.op()] + " " + right, e
	}
	p, e := b.property(c.Alias, c.Column)
	if e != nil {
		return "", e
	}
	op := operators[c.op()]
	param := "@" + name
	switch c.op() {
	case In, NotIn:
		suffix := ""
		if c.PartialMatch {
			suffix = ", true"
		}
		return op + "(" + p + ", " + param + suffix + ")", nil
	case Contains, NotContains:
		return op + "(" + param + ", " + p + ")", nil
	case Len:
		return op + "(" + p + ") = " + param, nil
	case Defined, NotDefined, IsNull, IsString:
		return op + "(" + p + ")", nil
	case StartsWith, StringContains, StringEquals:
		suffix := ""
		if c.IgnoreCase {
			suffix = ", true"
		}
		return op + "(" + p + ", " + param + suffix + ")", nil
	case FTContains, NotFTContains, FTContainsAll, NotFTContainsAll, FTContainsAny, NotFTContainsAny:
		return op + "(" + p + ", " + param + ")", nil
	}
	return p + " " + op + " " + param, nil
}
