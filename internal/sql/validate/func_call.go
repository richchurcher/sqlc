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

	// Custom validation for sqlc.arg, sqlc.narg and sqlc.slice
	// TODO: Replace this once type-checking is implemented
	if fn.Schema == "sqlc" {
		if !(fn.Name == "arg" || fn.Name == "narg" || fn.Name == "slice" || fn.Name == "embed") {
			v.err = sqlerr.FunctionNotFound("sqlc." + fn.Name)
			return nil
		}

		nArgs := len(call.Args.Items)
		if nArgs < 1 {
			v.err = &sqlerr.Error{
				Message:  fmt.Sprintf("expected 1 parameter to sqlc.%s; got %d", fn.Name, len(call.Args.Items)),
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
		if nArgs == 2 && fn.Name == "embed" {
			// TODO: fix
			option := "nullable"
			if option != "nullable" {
				v.err = &sqlerr.Error{
					Message:  fmt.Sprintf("expected option to sqlc.%s to be `nullable`, got %s", fn.Name, option),
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
