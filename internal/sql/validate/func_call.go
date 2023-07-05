package validate

import (
	"errors"
	"fmt"

	"github.com/kyleconroy/sqlc/internal/config"
	"github.com/kyleconroy/sqlc/internal/sql/ast"
	"github.com/kyleconroy/sqlc/internal/sql/astutils"
	"github.com/kyleconroy/sqlc/internal/sql/catalog"
	"github.com/kyleconroy/sqlc/internal/sql/sqlerr"
)

type funcCallVisitor struct {
	catalog  *catalog.Catalog
	settings config.CombinedSettings
	err      error
}

func (v *funcCallVisitor) Visit(node ast.Node) astutils.Visitor {
	if v.err != nil {
		return nil
	}

	call, ok := node.(*ast.FuncCall)
	if !ok {
		return v
	}
	fn := call.Func
	if fn == nil {
		return v
	}

	// Custom validation for `sqlc.` functions
	// TODO: Replace this once type-checking is implemented
	if fn.Schema == "sqlc" {
		if !(fn.Name == "arg" || fn.Name == "narg" || fn.Name == "slice" || fn.Name == "embed") {
			v.err = sqlerr.FunctionNotFound("sqlc." + fn.Name)
			return nil
		}

		minArgCount := 1
		maxArgCount := 1
		if fn.Name == "embed" {
			maxArgCount = 2
		}

		if len(call.Args.Items) < minArgCount {
			v.err = &sqlerr.Error{
				Message:  fmt.Sprintf("expected at least %d parameter(s) to sqlc.%s; got %d", minArgCount, fn.Name, len(call.Args.Items)),
				Location: call.Pos(),
			}
			return nil
		}

		if len(call.Args.Items) > maxArgCount {
			v.err = &sqlerr.Error{
				Message:  fmt.Sprintf("expected at most %d parameter(s) to sqlc.%s; got %d", maxArgCount, fn.Name, len(call.Args.Items)),
				Location: call.Pos(),
			}
			return nil
		}

		switch n := call.Args.Items[0].(type) {
		case *ast.A_Const:
		case *ast.ColumnRef:
		default:
			v.err = &sqlerr.Error{
				Message:  fmt.Sprintf("expected parameter to sqlc.%s to be string or reference; got %T", fn.Name, n),
				Location: call.Pos(),
			}
			return nil
		}

		// This is obviously not a flexible solution, but likely to go away entirely when the
		// rest of this block does. Meanwhile, check for the only valid use of the second argument.
		if len(call.Args.Items) == 2 {
			option := call.Args.Items[1]
			switch n := option.(type) {
			case *ast.A_Const:
				val := option.(*ast.A_Const).Val
				if str, ok := val.(*ast.String); ok {
					if str.Str != "nullable" {
						v.err = &sqlerr.Error{
							Message:  fmt.Sprintf("valid options for sqlc.%s are: `nullable`, got %s", fn.Name, str),
							Location: call.Pos(),
						}
						return nil
					}
				} else {
					v.err = &sqlerr.Error{
						Message:  fmt.Sprintf("options for sqlc.%s must be string", fn.Name),
						Location: call.Pos(),
					}
					return nil
				}
			default:
				v.err = &sqlerr.Error{
					Message:  fmt.Sprintf("expected parameter to sqlc.%s to be string or reference; got %T (%#v)", fn.Name, n, n.(*ast.ColumnRef)),
					Location: call.Pos(),
				}
				return nil
			}
		}

		// If we have sqlc.arg or sqlc.narg, there is no need to resolve the function call.
		// It won't resolve anyway, since it is not a real function.
		return nil
	}

	fun, err := v.catalog.ResolveFuncCall(call)
	if fun != nil {
		return v
	}
	if errors.Is(err, sqlerr.NotFound) && !v.settings.Package.StrictFunctionChecks {
		return v
	}
	v.err = err
	return nil
}

func FuncCall(c *catalog.Catalog, cs config.CombinedSettings, n ast.Node) error {
	visitor := funcCallVisitor{catalog: c, settings: cs}
	astutils.Walk(&visitor, n)
	return visitor.err
}
