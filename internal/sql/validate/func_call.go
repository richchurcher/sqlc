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
			case *ast.ColumnRef:
				vals := option.(*ast.ColumnRef).Fields.Items
				fmt.Printf("DEBUG: len(vals) :: %d\n", len(vals))
				// if str, ok := val.(*ast.String); ok {
				// 	fmt.Printf("DEBUG: str :: %s", str)
				// 	if str.Str != "nullable" {
				//
				// 		v.err = &sqlerr.Error{
				// 			Message:  fmt.Sprintf("valid options for sqlc.%s are: `nullable`, got %s", fn.Name, str),
				// 			Location: call.Pos(),
				// 		}
				// 		return nil
				// 	}
				// } else {
				// 	v.err = &sqlerr.Error{
				// 		Message:  fmt.Sprintf("options for sqlc.%s must be string", fn.Name),
				// 		Location: call.Pos(),
				// 	}
				// 	return nil
				// }
			default:
				fields := n.(*ast.ColumnRef).Fields
				v.err = &sqlerr.Error{
					// TODO: fix
					Message:  fmt.Sprintf("expected parameter to sqlc.%s to be string or reference; got %T (%#v)", fn.Name, n, fields.Items[0].(*ast.String)),
					Location: call.Pos(),
				}
				return nil
			}
		}

		// Don't try to resolve `sqlc.` functions.
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
