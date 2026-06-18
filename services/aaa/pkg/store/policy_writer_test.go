package store

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type recordedPolicyExec struct {
	sql  string
	args []any
}

type recordingPolicyRunner struct {
	execs []recordedPolicyExec
	tag   pgconn.CommandTag
	err   error
}

func (r *recordingPolicyRunner) Exec(_ context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	r.execs = append(r.execs, recordedPolicyExec{sql: sql, args: arguments})
	if r.tag.String() == "" {
		r.tag = pgconn.NewCommandTag("INSERT 0 1")
	}
	return r.tag, r.err
}

func TestUpsertManagedRolePermissionOwnsConfigMetadata(t *testing.T) {
	runner := &recordingPolicyRunner{}
	err := UpsertManagedRolePermission(context.Background(), runner, RolePermission{
		RoleName:     "release-manager",
		ResourceType: "pipeline",
		ResourceID:   "prod/deploy",
		Action:       "pipeline.execute",
		Effect:       "allow",
	}, ConfigMetadata{RepoID: 42, SourcePath: "access/grants.yaml", CommitSHA: "abc123"})
	if err != nil {
		t.Fatalf("UpsertManagedRolePermission() error = %v", err)
	}
	if len(runner.execs) != 1 {
		t.Fatalf("exec count = %d, want 1", len(runner.execs))
	}
	exec := runner.execs[0]
	if !strings.Contains(exec.sql, "INSERT INTO auth_role_permissions") {
		t.Fatalf("sql = %q, want auth_role_permissions insert", exec.sql)
	}
	if !strings.Contains(exec.sql, "managed_by_config_repo = TRUE") {
		t.Fatalf("sql = %q, want managed config update", exec.sql)
	}
	wantArgs := []any{"release-manager", "pipeline", "prod/deploy", "pipeline.execute", "allow", int64(42), "access/grants.yaml", "abc123"}
	if !sameArgs(exec.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", exec.args, wantArgs)
	}
}

func TestUpsertResourceACLAllowsGrantScopedAndServiceScopedRows(t *testing.T) {
	runner := &recordingPolicyRunner{}
	grantID := int64(7)
	err := UpsertResourceACL(context.Background(), runner, ResourceACL{
		ResourceType:  "pipeline",
		ResourceID:    "team-1/deploy",
		SubjectType:   "user",
		SubjectID:     "user-1",
		AccessGrantID: &grantID,
		Action:        "pipeline.read",
		Effect:        "allow",
	})
	if err != nil {
		t.Fatalf("UpsertResourceACL(grant-scoped) error = %v", err)
	}
	err = UpsertResourceACL(context.Background(), runner, ResourceACL{
		ResourceType: "scope",
		ResourceID:   "prod",
		SubjectType:  "service_account",
		SubjectID:    "schedule:nightly",
		Action:       "scope.use",
		Effect:       "allow",
	})
	if err != nil {
		t.Fatalf("UpsertResourceACL(service-scoped) error = %v", err)
	}
	if len(runner.execs) != 2 {
		t.Fatalf("exec count = %d, want 2", len(runner.execs))
	}
	if !strings.Contains(runner.execs[0].sql, "INSERT INTO resource_acl") {
		t.Fatalf("sql = %q, want resource_acl insert", runner.execs[0].sql)
	}
	if !strings.Contains(runner.execs[0].sql, "DO UPDATE SET access_grant_id = EXCLUDED.access_grant_id") {
		t.Fatalf("sql = %q, want access_grant_id ownership refresh", runner.execs[0].sql)
	}
	if runner.execs[0].args[4] != grantID {
		t.Fatalf("grant-scoped access_grant_id arg = %#v, want %d", runner.execs[0].args[4], grantID)
	}
	if runner.execs[1].args[4] != nil {
		t.Fatalf("service-scoped access_grant_id arg = %#v, want nil", runner.execs[1].args[4])
	}
}

func TestDeleteRoleBindingReturnsCommandTag(t *testing.T) {
	runner := &recordingPolicyRunner{tag: pgconn.NewCommandTag("DELETE 1")}
	tag, err := DeleteRoleBinding(context.Background(), runner, RoleBinding{
		RoleName:    "developer",
		SubjectType: "user",
		SubjectID:   "user-1",
	})
	if err != nil {
		t.Fatalf("DeleteRoleBinding() error = %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("RowsAffected() = %d, want 1", tag.RowsAffected())
	}
	if len(runner.execs) != 1 || !strings.Contains(runner.execs[0].sql, "DELETE FROM auth_role_bindings") {
		t.Fatalf("execs = %#v, want auth_role_bindings delete", runner.execs)
	}
}

func TestEnsurePolicyConfigMetadataSchemaOwnsGitOpsColumns(t *testing.T) {
	runner := &recordingPolicyRunner{}
	if err := EnsurePolicyConfigMetadataSchema(context.Background(), runner); err != nil {
		t.Fatalf("EnsurePolicyConfigMetadataSchema() error = %v", err)
	}

	if !recordedSQLContains(runner.execs, "ALTER TABLE auth_roles ADD COLUMN IF NOT EXISTS config_repo_id BIGINT") {
		t.Fatalf("schema statements missing auth_roles config_repo_id column: %#v", runner.execs)
	}
	if !recordedSQLContains(runner.execs, "CREATE INDEX IF NOT EXISTS idx_auth_role_permissions_config_repo_id") {
		t.Fatalf("schema statements missing auth_role_permissions config index: %#v", runner.execs)
	}
	if !recordedSQLContains(runner.execs, "ADD CONSTRAINT auth_role_bindings_config_repo_id_fkey") {
		t.Fatalf("schema statements missing auth_role_bindings config FK: %#v", runner.execs)
	}
}

func recordedSQLContains(execs []recordedPolicyExec, fragment string) bool {
	for _, exec := range execs {
		if strings.Contains(exec.sql, fragment) {
			return true
		}
	}
	return false
}

func sameArgs(got, want []any) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
