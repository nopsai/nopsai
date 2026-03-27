package authz

import (
	"context"
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Enforcer struct {
	e *casbin.Enforcer
}

const modelText = `
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && r.dom == p.dom && keyMatch2(r.obj, p.obj) && regexMatch(r.act, p.act)
`

const policyTemplateRole = "__policy_template__"

func NewEnforcer(ctx context.Context, db *pgxpool.Pool) (*Enforcer, error) {
	m, err := model.NewModelFromString(modelText)
	if err != nil {
		return nil, fmt.Errorf("create casbin model: %w", err)
	}
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, fmt.Errorf("create casbin enforcer: %w", err)
	}
	en := &Enforcer{e: e}
	if err := en.LoadPolicies(ctx, db); err != nil {
		return nil, err
	}
	return en, nil
}

func (e *Enforcer) LoadPolicies(ctx context.Context, db *pgxpool.Pool) error {
	if e == nil || e.e == nil {
		return nil
	}
	e.e.ClearPolicy()
	rows, err := db.Query(ctx, `SELECT role, tenant_id, obj, act FROM role_permissions`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var role, tenantID, obj, act string
		if err := rows.Scan(&role, &tenantID, &obj, &act); err != nil {
			return err
		}
		if role == policyTemplateRole {
			continue
		}
		_, _ = e.e.AddPolicy(strings.TrimSpace(role), tenantID, strings.TrimSpace(obj), strings.TrimSpace(act))
	}
	return nil
}

func (e *Enforcer) EnforceRoles(roles []string, tenantID, obj, act string) bool {
	if e == nil || e.e == nil {
		return true
	}
	for _, role := range roles {
		ok, err := e.e.Enforce(role, tenantID, obj, act)
		if err == nil && ok {
			return true
		}
	}
	return false
}
