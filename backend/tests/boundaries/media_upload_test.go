package boundaries_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestMediaUploadKeepsCreationSQLBehindOwnerPort(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate module")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	for _, name := range []string{"upload.go", "upload_admission.go"} {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "internal/media/adapter/postgres", name), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, imported := range file.Imports {
				value, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(value, "/internal/workspace/") {
					t.Errorf("upload adapter imports Creation implementation: %s", value)
				}
			}
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(strings.ToLower(value), "workspace.") {
					t.Errorf("upload adapter accesses Creation schema directly: %s", value)
				}
				return true
			})
		})
	}
}
