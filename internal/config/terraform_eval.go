package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// tfKind is the small set of Terraform values the env importer understands.
type tfKind int

const (
	tfUnknown tfKind = iota
	tfNull
	tfString
	tfNumber
	tfBool
	tfMap
)

// tfValue is a Terraform value reduced to what Cloud Run env needs.
// Anything the importer cannot evaluate stays unknown and is skipped.
type tfValue struct {
	kind tfKind
	str  string
	b    bool
	mp   map[string]tfValue
}

func tfStringVal(s string) tfValue {
	return tfValue{kind: tfString, str: s}
}

func tfNumberVal(s string) tfValue {
	return tfValue{kind: tfNumber, str: s}
}

func (v tfValue) isUnset() bool {
	return v.kind == tfUnknown || v.kind == tfNull || (v.kind == tfString && strings.TrimSpace(v.str) == "")
}

func (v tfValue) asString() (string, bool) {
	if v.kind != tfString {
		return "", false
	}
	return v.str, true
}

// tfScope evaluates expressions against variable values and lazy locals.
type tfScope struct {
	vars       map[string]tfValue
	localExprs map[string]hcl.Expression
	locals     map[string]tfValue
	visiting   map[string]bool
}

func newTFScope() *tfScope {
	return &tfScope{
		vars:       map[string]tfValue{},
		localExprs: map[string]hcl.Expression{},
		locals:     map[string]tfValue{},
		visiting:   map[string]bool{},
	}
}

func (s *tfScope) setVar(name string, v tfValue) {
	if s.vars == nil {
		s.vars = map[string]tfValue{}
	}
	s.vars[name] = v
}

func (s *tfScope) local(name string) tfValue {
	if v, ok := s.locals[name]; ok {
		return v
	}
	expr, ok := s.localExprs[name]
	if !ok {
		return tfValue{}
	}
	if s.visiting[name] {
		return tfValue{}
	}
	s.visiting[name] = true
	v := s.eval(expr)
	delete(s.visiting, name)
	s.locals[name] = v
	return v
}

func (s *tfScope) eval(expr hcl.Expression) tfValue {
	if expr == nil || s == nil {
		return tfValue{}
	}
	switch e := expr.(type) {
	case *hclsyntax.ParenthesesExpr:
		return s.eval(e.Expression)
	case *hclsyntax.TemplateWrapExpr:
		return s.eval(e.Wrapped)
	case *hclsyntax.TemplateExpr:
		return s.evalTemplate(e)
	case *hclsyntax.LiteralValueExpr:
		return valueFromCty(e.Val)
	case *hclsyntax.ScopeTraversalExpr:
		return s.evalTraversal(e.Traversal)
	case *hclsyntax.ObjectConsKeyExpr:
		return s.evalObjectKey(e)
	case *hclsyntax.ObjectConsExpr:
		return s.evalObject(e)
	case *hclsyntax.ConditionalExpr:
		return s.evalConditional(e)
	case *hclsyntax.BinaryOpExpr:
		return s.evalBinary(e)
	case *hclsyntax.FunctionCallExpr:
		return s.evalCall(e)
	default:
		return tfValue{}
	}
}

func (s *tfScope) evalTemplate(e *hclsyntax.TemplateExpr) tfValue {
	var b strings.Builder
	for _, part := range e.Parts {
		text, ok := s.eval(part).templateString()
		if !ok {
			return tfValue{}
		}
		b.WriteString(text)
	}
	return tfStringVal(b.String())
}

func (v tfValue) templateString() (string, bool) {
	switch v.kind {
	case tfString, tfNumber:
		return v.str, true
	case tfBool:
		if v.b {
			return "true", true
		}
		return "false", true
	default:
		return "", false
	}
}

func (s *tfScope) evalObjectKey(e *hclsyntax.ObjectConsKeyExpr) tfValue {
	if !e.ForceNonLiteral {
		if name, ok := identifierName(e.Wrapped); ok {
			return tfStringVal(name)
		}
		if lit, ok := e.Wrapped.(*hclsyntax.LiteralValueExpr); ok {
			if lit.Val.IsNull() {
				return tfStringVal("null")
			}
			if lit.Val.Type().Equals(cty.Bool) {
				if lit.Val.True() {
					return tfStringVal("true")
				}
				return tfStringVal("false")
			}
		}
	}
	return s.eval(e.Wrapped)
}

func (s *tfScope) evalObject(e *hclsyntax.ObjectConsExpr) tfValue {
	out := make(map[string]tfValue, len(e.Items))
	for _, item := range e.Items {
		key, ok := s.eval(item.KeyExpr).asString()
		if !ok || key == "" {
			continue
		}
		out[key] = s.eval(item.ValueExpr)
	}
	return tfValue{kind: tfMap, mp: out}
}

func (s *tfScope) evalConditional(e *hclsyntax.ConditionalExpr) tfValue {
	cond := s.eval(e.Condition)
	if cond.kind != tfBool {
		return tfValue{}
	}
	if cond.b {
		return s.eval(e.TrueResult)
	}
	return s.eval(e.FalseResult)
}

func (s *tfScope) evalBinary(e *hclsyntax.BinaryOpExpr) tfValue {
	if e.Op != hclsyntax.OpEqual {
		return tfValue{}
	}
	lhs := s.eval(e.LHS)
	rhs := s.eval(e.RHS)
	if lhs.kind != tfString || rhs.kind != tfString {
		return tfValue{}
	}
	return tfValue{kind: tfBool, b: lhs.str == rhs.str}
}

func (s *tfScope) evalCall(e *hclsyntax.FunctionCallExpr) tfValue {
	if e.ExpandFinal {
		return tfValue{}
	}
	switch e.Name {
	case "merge":
		return s.evalMerge(e.Args)
	case "lookup":
		return s.evalLookup(e.Args)
	case "upper", "lower":
		if len(e.Args) != 1 {
			return tfValue{}
		}
		text, ok := s.eval(e.Args[0]).asString()
		if !ok {
			return tfValue{}
		}
		if e.Name == "upper" {
			return tfStringVal(strings.ToUpper(text))
		}
		return tfStringVal(strings.ToLower(text))
	case "tostring":
		if len(e.Args) != 1 {
			return tfValue{}
		}
		return s.eval(e.Args[0]).asToString()
	default:
		return tfValue{}
	}
}

func (v tfValue) asToString() tfValue {
	switch v.kind {
	case tfString:
		return v
	case tfNumber:
		return tfStringVal(v.str)
	case tfBool:
		if v.b {
			return tfStringVal("true")
		}
		return tfStringVal("false")
	default:
		return tfValue{}
	}
}

// evalMerge overlays object arguments. Unknown arguments are skipped so a known
// default map still contributes when a later argument cannot be evaluated.
func (s *tfScope) evalMerge(args []hclsyntax.Expression) tfValue {
	out := map[string]tfValue{}
	known := false
	for _, arg := range args {
		val := s.eval(arg)
		if val.kind != tfMap {
			continue
		}
		known = true
		for key, item := range val.mp {
			out[key] = item
		}
	}
	if !known {
		return tfValue{}
	}
	return tfValue{kind: tfMap, mp: out}
}

func (s *tfScope) evalLookup(args []hclsyntax.Expression) tfValue {
	if len(args) < 2 {
		return tfValue{}
	}
	key, keyOK := s.eval(args[1]).asString()
	obj := s.eval(args[0])
	if keyOK && obj.kind == tfMap {
		if val, ok := obj.mp[key]; ok {
			return val
		}
	}
	if len(args) >= 3 {
		return s.eval(args[2])
	}
	return tfValue{}
}

func (s *tfScope) evalTraversal(trav hcl.Traversal) tfValue {
	if len(trav) == 0 {
		return tfValue{}
	}
	root, ok := trav[0].(hcl.TraverseRoot)
	if !ok || len(trav) < 2 {
		return tfValue{}
	}
	attr, ok := trav[1].(hcl.TraverseAttr)
	if !ok || attr.Name == "" {
		return tfValue{}
	}
	var cur tfValue
	switch root.Name {
	case "var":
		cur = s.vars[attr.Name]
	case "local":
		cur = s.local(attr.Name)
	default:
		return tfValue{}
	}
	for _, step := range trav[2:] {
		next, ok := step.(hcl.TraverseAttr)
		if !ok || cur.kind != tfMap {
			return tfValue{}
		}
		cur, ok = cur.mp[next.Name]
		if !ok {
			return tfValue{}
		}
	}
	return cur
}

func valueFromCty(v cty.Value) tfValue {
	if v.IsNull() {
		return tfValue{kind: tfNull}
	}
	if !v.IsKnown() {
		return tfValue{}
	}
	switch {
	case v.Type().Equals(cty.String):
		return tfStringVal(v.AsString())
	case v.Type().Equals(cty.Number):
		text, ok := numberString(v)
		if !ok {
			return tfValue{}
		}
		return tfNumberVal(text)
	case v.Type().Equals(cty.Bool):
		return tfValue{kind: tfBool, b: v.True()}
	default:
		return tfValue{}
	}
}

func numberString(v cty.Value) (string, bool) {
	bf := v.AsBigFloat()
	if bf == nil {
		return "", false
	}
	if bf.IsInt() {
		return bf.Text('f', 0), true
	}
	return bf.Text('f', -1), true
}

// loadTFVars reads assignments from a .tfvars file.
// Values that do not evaluate are omitted. The ws.yaml environment name is not consulted.
func loadTFVars(path string) (map[string]tfValue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read terraform var file %q: %w", path, err)
	}
	body, err := parseHCLBody(data, path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse terraform var file: %w", err)
	}
	scope := newTFScope()
	out := map[string]tfValue{}
	for name, attr := range body.Attributes {
		val := scope.eval(attr.Expr)
		if val.kind == tfUnknown || val.kind == tfNull {
			continue
		}
		out[name] = val
	}
	return out, nil
}
