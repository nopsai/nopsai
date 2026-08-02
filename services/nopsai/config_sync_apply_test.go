package nopsai

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestApplyConfigSyncPlanSyncsTeamsBeforeTeamOwnedResources(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "config_sync_apply.go", nil, 0)
	if err != nil {
		t.Fatalf("failed to parse config_sync_apply.go: %v", err)
	}

	var applyFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "applyConfigSyncPlan" {
			applyFunc = fn
			break
		}
	}
	if applyFunc == nil {
		t.Fatal("applyConfigSyncPlan not found")
	}

	var teamSync, dashboardApply, notificationApply token.Pos
	teamSyncCount := 0
	ast.Inspect(applyFunc.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch callName(call.Fun) {
		case "syncPipelineRunTeams":
			teamSyncCount++
			teamSync = call.Pos()
		case "applyGitOpsDashboards":
			dashboardApply = call.Pos()
		case "sortedNotificationRoutes":
			notificationApply = call.Pos()
		}
		return true
	})

	if teamSync == token.NoPos {
		t.Fatal("applyConfigSyncPlan does not sync pipeline run teams")
	}
	if teamSyncCount != 1 {
		t.Fatal("applyConfigSyncPlan should sync pipeline run teams exactly once")
	}
	if dashboardApply == token.NoPos {
		t.Fatal("applyConfigSyncPlan does not apply GitOps dashboards")
	}
	if notificationApply == token.NoPos {
		t.Fatal("applyConfigSyncPlan does not apply notification routes")
	}
	if teamSync > dashboardApply {
		t.Fatal("pipeline run teams must sync before GitOps dashboards resolve team paths")
	}
	if teamSync > notificationApply {
		t.Fatal("pipeline run teams must sync before notification routes resolve team paths")
	}
}

func callName(expr ast.Expr) string {
	switch fun := expr.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	default:
		return ""
	}
}
