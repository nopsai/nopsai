package discovery

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Registration struct {
	Method string
	Path   string
}

func Discover(root string) ([]Registration, error) {
	serviceDir := filepath.Join(root, "services", "nopsai")
	seen := make(map[Registration]struct{})
	err := filepath.WalkDir(serviceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "HandleFunc" {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			pattern, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			method, routePath, ok := strings.Cut(pattern, " ")
			if !ok || method != strings.ToUpper(method) || !strings.HasPrefix(routePath, "/") {
				return true
			}
			seen[Registration{Method: method, Path: routePath}] = struct{}{}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover API routes: %w", err)
	}
	routes := make([]Registration, 0, len(seen))
	for route := range seen {
		routes = append(routes, route)
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return routes, nil
}
