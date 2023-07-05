package rewrite

import (
	"fmt"

	"github.com/kyleconroy/sqlc/internal/sql/ast"
	"github.com/kyleconroy/sqlc/internal/sql/astutils"
)

// Embed is an instance of `sqlc.embed(param)`
type Embed struct {
	Table  *ast.TableName
	param  string
	Node   *ast.ColumnRef
	option string
}

// Orig string to replace
func (e Embed) Orig() string {
	return fmt.Sprintf("sqlc.embed(%s, %s)", e.param, e.option)
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
// `param.*` column refs to produce the correct source edits. If an optional
// `nullable` is passed as a second parameter, embed attempts to use a nil
// pointer as the embedded type.
func Embeds(raw *ast.RawStmt) (*ast.RawStmt, EmbedSet) {
	var embeds []*Embed

	node := astutils.Apply(raw, func(cr *astutils.Cursor) bool {
		node := cr.Node()

		switch {
		case isEmbed(node):
			fun := node.(*ast.FuncCall)
			fmt.Printf("DEBUG: -- Embed -- fun :: %#v\n", fun)
			nArgs := len(fun.Args.Items)
			fmt.Printf("DEBUG: -- Embed -- nArgs :: %#v\n", nArgs)

			if nArgs < 1 || nArgs > 2 {
				return false
			}

			fmt.Printf("DEBUG: -- Embed -- args :: %#v\n", fun.Args)
			param, _ := flatten(fun.Args)

			node := &ast.ColumnRef{
				Fields: &ast.List{
					Items: []ast.Node{
						&ast.String{Str: param},
						&ast.A_Star{},
					},
				},
			}

			embeds = append(embeds, &Embed{
				Table: &ast.TableName{Name: param},
				param: param,
				Node:  node,
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
