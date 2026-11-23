package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// inputWriters are the Windows APIs that create input in software. The app
// sends all input through the Board instead (docs/adr/0001).
var inputWriters = map[string]bool{
	"SendInput":            true,
	"keybd_event":          true,
	"mouse_event":          true,
	"SetCursorPos":         true,
	"SetPhysicalCursorPos": true,
	"PostMessage":          true,
	"PostMessageA":         true,
	"PostMessageW":         true,
	"BlockInput":           true,
}

// inputWriterUses lists each identifier or string literal in src that names
// an input-writing API, such as w32.SendInput or NewProc("SendInput").
// Comments do not count.
func inputWriterUses(fset *token.FileSet, name string, src any) ([]string, error) {
	file, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var uses []string
	ast.Inspect(file, func(n ast.Node) bool {
		var found string
		switch n := n.(type) {
		case *ast.Ident:
			found = n.Name
		case *ast.BasicLit:
			if n.Kind == token.STRING {
				found, _ = strconv.Unquote(n.Value)
			}
		default:
			return true
		}
		if inputWriters[found] {
			uses = append(uses, fset.Position(n.Pos()).String()+": "+found)
		}
		return true
	})
	return uses, nil
}

func TestNoSoftwareInput(t *testing.T) {
	fset := token.NewFileSet()
	checked := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != "." && (strings.HasPrefix(name, ".") || name == "frontend" || name == "testdata" ||
				name == "datasets" || name == "fishing-recordings") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		uses, err := inputWriterUses(fset, path, nil)
		if err != nil {
			return err
		}
		checked++
		for _, use := range uses {
			t.Errorf("%s creates input in software; send it through the Board (docs/adr/0001)", use)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 50 {
		t.Fatalf("checked only %d Go files; is the walk skipping the module?", checked)
	}
}

func TestInputWriterUsesIgnoresComments(t *testing.T) {
	src := `package x
// A program's SendInput is marked injected.
var p = user32.NewProc("PostMessageW")
func f() { w32.SendInput(0, nil, 0) }
`
	uses, err := inputWriterUses(token.NewFileSet(), "x.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) != 2 {
		t.Fatalf("uses = %q, want the NewProc string and the w32 call", uses)
	}
}
