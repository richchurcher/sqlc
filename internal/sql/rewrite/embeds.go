package rewrite

import (
	"fmt"

	"github.com/kyleconroy/sqlc/internal/sql/ast"
	"github.com/kyleconroy/sqlc/internal/sql/astutils"
)

// Embed is an instance of `sqlc.embed(param)`
type Embed struct {
	Table    *ast.TableName
	param    string
	Node     *ast.ColumnRef
	nullable bool
}

// Orig string to replace
func (e Embed) Orig() string {
	call := "sqlc.embed(%s)"
	if e.nullable {
		call = "sqlc.embed(%s, nullable)"
	}
	return fmt.Sprintf(call, e.param)
}

// EmbedSet is a set of Embed instances
type EmbedSet []*Embed

// Find a matching embed by column ref
func (es EmbedSet) Find(node *ast.ColumnRef) (*Embed, bool) {
	for _, e := range es {
		if e.Node == node {
			return e, true
		}
	}
	return nil, false
}

// Embeds rewrites `sqlc.embed(param)` to a `ast.ColumnRef` of form `param.*`.
// The compiler can make use of the returned `EmbedSet` while expanding the
// `param.*` column refs to produce the correct source edits. An optional
// second parameter, `nullable`, can indicate that the column may be the result
// of a LEFT JOIN (or otherwise NULL) allowing the compiler to generate a nil pointer.
func Embeds(raw *ast.RawStmt) (*ast.RawStmt, EmbedSet) {
	var embeds []*Embed

	node := astutils.Apply(raw, func(cr *astutils.Cursor) bool {
		node := cr.Node()

		switch {
		case isEmbed(node):
			fun := node.(*ast.FuncCall)
			nArgs := len(fun.Args.Items)

			if nArgs == 0 {
				return false
			}

			var option *string
			if nArgs == 2 {
				o, _ := flatten(fun.Args.Items[1])
				if o == "nullable" {
					option = &o
				}
			}
			param, _ := flatten(fun.Args.Items[0])

			node := &ast.ColumnRef{
				Fields: &ast.List{
					Items: []ast.Node{
						&ast.String{Str: param},
						&ast.A_Star{},
					},
				},
			}

			nullable := option != nil
			embeds = append(embeds, &Embed{
				Table:    &ast.TableName{Name: param},
				param:    param,
				Node:     node,
				nullable: nullable,
			})

			cr.Replace(node)
			return false
		default:
			return true
		}
	}, nil)

	return node.(*ast.RawStmt), embeds
}

func isEmbed(node ast.Node) bool {
	call, ok := node.(*ast.FuncCall)
	if !ok {
		return false
	}

	if call.Func == nil {
		return false
	}

	isValid := call.Func.Schema == "sqlc" && call.Func.Name == "embed"
	return isValid
}
