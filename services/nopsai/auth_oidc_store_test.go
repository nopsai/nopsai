package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type recordedOIDCExec struct {
	sql  string
	args []any
}

type recordingOIDCTx struct {
	execs []recordedOIDCExec
}

func (tx *recordingOIDCTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tx.execs = append(tx.execs, recordedOIDCExec{sql: sql, args: args})
	return pgconn.NewCommandTag("INSERT 1"), nil
}

func (tx *recordingOIDCTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("unexpected query")
}

func (tx *recordingOIDCTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	tx.execs = append(tx.execs, recordedOIDCExec{sql: sql, args: args})
	if strings.Contains(sql, "RETURNING id") {
		return recordingOIDCRow{values: []any{int64(42)}}
	}
	return recordingOIDCRow{values: []any{"sso-owner@example.com"}}
}

type recordingOIDCRow struct {
	values []any
	err    error
}

func (r recordingOIDCRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		if i >= len(r.values) {
			break
		}
		switch target := dest[i].(type) {
		case *int64:
			if value, ok := r.values[i].(int64); ok {
				*target = value
			}
		case *string:
			if value, ok := r.values[i].(string); ok {
				*target = value
			}
		}
	}
	return nil
}

func TestSyncOIDCAuthTeamMembershipsCreatesAndPrunesProviderTeams(t *testing.T) {
	ctx := context.Background()
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	tx := &recordingOIDCTx{}

	err := syncOIDCAuthTeamMemberships(ctx, tx, userID, "nopsai", map[string]string{
		"keycloak-admins": "NopsAI Admins",
		"keycloak-devs":   "NopsAI Developers",
	})
	if err != nil {
		t.Fatalf("syncOIDCAuthTeamMemberships() error = %v", err)
	}

	if !recordedOIDCExecContains(tx.execs, "INSERT INTO auth_teams", "NopsAI Admins", "nopsai", "keycloak-admins") {
		t.Fatalf("execs missing auth team upsert for admins: %#v", tx.execs)
	}
	if !recordedOIDCExecContains(tx.execs, "auth_team_name <>", "keycloak-admins", "NopsAI Admins") {
		t.Fatalf("execs missing stale remap prune for admins: %#v", tx.execs)
	}
	if !recordedOIDCExecContains(tx.execs, "external_group_id, ''), external_team_name) = ANY", "nopsai") {
		t.Fatalf("execs missing provider-managed stale team prune: %#v", tx.execs)
	}
	if recordedOIDCExecContains(tx.execs, "managed_by_identity_provider = FALSE", userID.String()) {
		t.Fatalf("execs should not prune local team memberships: %#v", tx.execs)
	}
}

func TestSyncOIDCAuthTeamMembershipsPrunesWhenMappingEmpty(t *testing.T) {
	ctx := context.Background()
	userID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	tx := &recordingOIDCTx{}

	if err := syncOIDCAuthTeamMemberships(ctx, tx, userID, "nopsai", nil); err != nil {
		t.Fatalf("syncOIDCAuthTeamMemberships() error = %v", err)
	}

	if !recordedOIDCExecContains(tx.execs, "provider_id, ''), identity_provider_id) = $2", "nopsai") {
		t.Fatalf("execs missing provider-managed team prune: %#v", tx.execs)
	}
	if recordedOIDCExecContains(tx.execs, "managed_by_identity_provider = FALSE", userID.String()) {
		t.Fatalf("execs should not prune local team memberships: %#v", tx.execs)
	}
}

func TestOIDCBasicRoleGrantSetForTeamsUsesStrongestRolePerTarget(t *testing.T) {
	got := oidcBasicRoleGrantSetForTeams(map[string]oidcBasicRoleGrantMapping{
		"team-1-viewer": {
			Role:     "viewer",
			Resource: "team:team-1",
		},
		"team-1-owner": {
			Role:         "owner",
			ResourceType: "team",
			ResourceID:   "team-1",
		},
		"platform-admin": {
			Role:     "admin",
			Resource: "platform:default",
		},
	}, []string{"team-1-viewer", "team-1-owner", "platform-admin"})

	if len(got) != 1 {
		t.Fatalf("grant set length = %d, want 1: %#v", len(got), got)
	}
	grant := got["team:team-1"]
	if grant.Role != productRoleOwner || grant.ResourceType != grantResourceTeam || grant.ResourceID != "team-1" || grant.ExternalTeam != "team-1-owner" {
		t.Fatalf("grant = %#v, want owner team:team-1 from team-1-owner", grant)
	}
}

func TestOIDCDesiredAccessRoleSetDoesNotAddImplicitViewer(t *testing.T) {
	provider := oidcProviderRecord{
		RoleMapping: map[string]string{
			"platform-admins": defaultAdminRole,
		},
	}

	got := oidcDesiredAccessRoleSet(provider, oidcSettings{}, oidcVerifiedIdentity{
		Teams:       []string{"team-1"},
		AccessRoles: []string{"ignored"},
	})
	if len(got) != 0 {
		t.Fatalf("roles = %#v, want no implicit viewer or unknown direct roles", got)
	}

	got = oidcDesiredAccessRoleSet(provider, oidcSettings{DefaultRole: " developer "}, oidcVerifiedIdentity{
		Teams:       []string{"platform-admins"},
		AccessRoles: []string{"Owner"},
	})
	for _, role := range []string{productRoleDeveloper, defaultAdminRole, productRoleOwner} {
		if !got[role] {
			t.Fatalf("roles = %#v, missing %q", got, role)
		}
	}
	if got[productRoleViewer] {
		t.Fatalf("roles = %#v, should not add viewer unless explicitly configured or mapped", got)
	}
}

func TestOIDCSettingsRequestCanClearDefaultRole(t *testing.T) {
	var omitted oidcSettingsRequest
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
		t.Fatalf("json.Unmarshal(omitted) error = %v", err)
	}
	if omitted.DefaultRole != nil {
		t.Fatalf("omitted default role = %#v, want nil", omitted.DefaultRole)
	}

	var cleared oidcSettingsRequest
	if err := json.Unmarshal([]byte(`{"default_role":""}`), &cleared); err != nil {
		t.Fatalf("json.Unmarshal(cleared) error = %v", err)
	}
	if cleared.DefaultRole == nil || *cleared.DefaultRole != "" {
		t.Fatalf("cleared default role = %#v, want empty string pointer", cleared.DefaultRole)
	}
}

func TestOIDCBasicRoleGrantSetFromGrantsUsesStrongestRolePerTarget(t *testing.T) {
	got := oidcBasicRoleGrantSetFromGrants([]oidcDesiredBasicRoleGrant{
		{
			ExternalTeam:          "/team-1",
			Role:                  productRoleViewer,
			ResourceType:          grantResourceTeam,
			ResourceID:            "/team-1/",
			RequireResourceExists: true,
		},
		{
			ExternalTeam:          "/team-1",
			Role:                  productRoleOwner,
			ResourceType:          grantResourceTeam,
			ResourceID:            "team-1",
			RequireResourceExists: true,
		},
		{
			ExternalTeam: "/platform-admin",
			Role:         productRoleAdmin,
			ResourceType: grantResourceTeam,
			ResourceID:   "team-1",
		},
	})

	if len(got) != 1 {
		t.Fatalf("grant set length = %d, want 1: %#v", len(got), got)
	}
	grant := got["team:team-1"]
	if grant.Role != productRoleOwner || grant.ResourceID != "team-1" || !grant.Inherit || !grant.RequireResourceExists {
		t.Fatalf("grant = %#v, want strongest owner grant with normalized target and existence guard", grant)
	}
}

func TestSyncOIDCBasicRoleGrantsUpsertsExpandsAndPrunes(t *testing.T) {
	ctx := context.Background()
	userID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	tx := &recordingOIDCTx{}

	err := syncOIDCBasicRoleGrants(ctx, tx, userID, "nopsai", map[string]oidcDesiredBasicRoleGrant{
		"team:team-1": {
			ExternalTeam: "team-1-owner",
			Role:         productRoleOwner,
			ResourceType: grantResourceTeam,
			ResourceID:   "team-1",
			Inherit:      true,
		},
	})
	if err != nil {
		t.Fatalf("syncOIDCBasicRoleGrants() error = %v", err)
	}

	if !recordedOIDCExecContains(tx.execs, "INSERT INTO access_grants", userID.String(), "team-1-owner", "sso:nopsai") {
		t.Fatalf("execs missing provider-managed access grant upsert: %#v", tx.execs)
	}
	if !recordedOIDCExecContains(tx.execs, "INSERT INTO resource_acl", grantResourceTeam, "team-1", "pipeline.read") {
		t.Fatalf("execs missing expanded resource ACL: %#v", tx.execs)
	}
	if !recordedOIDCExecContains(tx.execs, "INSERT INTO resource_ownership", grantResourceTeam, "team-1", userID.String()) {
		t.Fatalf("execs missing owner expansion: %#v", tx.execs)
	}
	if !recordedOIDCExecContains(tx.execs, "NOT (id = ANY", "nopsai") {
		t.Fatalf("execs missing stale provider-managed grant prune: %#v", tx.execs)
	}
}

func TestSyncOIDCBasicRoleGrantsPrunesWhenEmpty(t *testing.T) {
	ctx := context.Background()
	userID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	tx := &recordingOIDCTx{}

	if err := syncOIDCBasicRoleGrants(ctx, tx, userID, "nopsai", nil); err != nil {
		t.Fatalf("syncOIDCBasicRoleGrants() error = %v", err)
	}

	if !recordedOIDCExecContains(tx.execs, "DELETE FROM access_grants", userID.String(), "nopsai") {
		t.Fatalf("execs missing provider-managed grant prune: %#v", tx.execs)
	}
}

func TestPruneSupersededExternalIdentitiesDeletesSameProviderEmailSubjects(t *testing.T) {
	ctx := context.Background()
	userID := uuid.MustParse("55555555-5555-4555-8555-555555555555")
	tx := &recordingOIDCTx{}

	err := pruneSupersededExternalIdentities(ctx, tx, userID, oidcProviderRecord{ID: "nopsai"}, oidcVerifiedIdentity{
		Issuer:  "http://keycloak.test/realms/nopsai",
		Subject: "new-subject",
		Email:   "jip@example.com",
	})
	if err != nil {
		t.Fatalf("pruneSupersededExternalIdentities() error = %v", err)
	}

	if !recordedOIDCExecContains(tx.execs, "DELETE FROM auth_external_identities", userID.String(), "nopsai", "http://keycloak.test/realms/nopsai", "jip@example.com", "new-subject") {
		t.Fatalf("execs missing scoped superseded identity prune: %#v", tx.execs)
	}
}

func recordedOIDCExecContains(execs []recordedOIDCExec, sqlFragment string, args ...string) bool {
	for _, exec := range execs {
		if !strings.Contains(exec.sql, sqlFragment) {
			continue
		}
		matchedArgs := true
		for _, want := range args {
			if !recordedOIDCArgsContain(exec.args, want) {
				matchedArgs = false
				break
			}
		}
		if matchedArgs {
			return true
		}
	}
	return false
}

func recordedOIDCArgsContain(args []any, want string) bool {
	for _, arg := range args {
		switch value := arg.(type) {
		case string:
			if value == want {
				return true
			}
		case uuid.UUID:
			if value.String() == want {
				return true
			}
		case []string:
			for _, item := range value {
				if item == want {
					return true
				}
			}
		}
	}
	return false
}
