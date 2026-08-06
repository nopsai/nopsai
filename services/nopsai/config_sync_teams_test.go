package nopsai

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nopsai/services/nopsai/internal/configsync"
)

func TestPipelineRunAuthTeamNamesForStructureUsesFullTeamPaths(t *testing.T) {
	got, err := pipelineRunAuthTeamNamesForStructure(map[string]*configsync.PipelineRunStructureNode{
		"engineering": {
			Children: map[string]*configsync.PipelineRunStructureNode{
				"platform": {
					Apps: []configsync.PipelineRunStructureApp{{Name: "service-api", RepoURL: "https://github.com/acme/service-api"}},
				},
				"service-owners": {},
			},
		},
		"platform": {
			Children: map[string]*configsync.PipelineRunStructureNode{
				"prod":           {},
				"infrastructure": {},
			},
		},
	})
	if err != nil {
		t.Fatalf("pipelineRunAuthTeamNamesForStructure() error = %v", err)
	}
	sort.Strings(got)

	want := []string{
		"engineering",
		"engineering/platform",
		"engineering/service-owners",
		"platform",
		"platform/infrastructure",
		"platform/prod",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("auth team names = %#v, want %#v", got, want)
	}
}

func TestLoadExistingTeamRecordsUsesSiblingScopedNames(t *testing.T) {
	parentID := 1
	runner := &teamRecordQueryRunner{rows: &teamRecordRows{rows: []teamRecordRow{
		{ID: 1, Name: "engineering", Kind: "team"},
		{ID: 2, Name: "platform", Kind: "team", ParentID: sql.NullInt32{Int32: int32(parentID), Valid: true}},
		{ID: 3, Name: "platform", Kind: "team"},
	}}}

	records, err := loadExistingTeamRecords(context.Background(), runner)
	if err != nil {
		t.Fatalf("loadExistingTeamRecords() error = %v", err)
	}
	if got := records.byParentName[teamSiblingKey("platform", nil)]; got == nil || got.ID != 3 {
		t.Fatalf("root platform record = %#v, want id 3", got)
	}
	if got := records.byParentName[teamSiblingKey("platform", &parentID)]; got == nil || got.ID != 2 {
		t.Fatalf("engineering/platform record = %#v, want id 2", got)
	}
}

func TestLoadExistingTeamRecordsRejectsDuplicateSiblingNames(t *testing.T) {
	parentID := 1
	runner := &teamRecordQueryRunner{rows: &teamRecordRows{rows: []teamRecordRow{
		{ID: 1, Name: "engineering", Kind: "team"},
		{ID: 2, Name: "platform", Kind: "team", ParentID: sql.NullInt32{Int32: int32(parentID), Valid: true}},
		{ID: 3, Name: "platform", Kind: "team", ParentID: sql.NullInt32{Int32: int32(parentID), Valid: true}},
	}}}

	_, err := loadExistingTeamRecords(context.Background(), runner)
	if err == nil {
		t.Fatal("loadExistingTeamRecords() error = nil, want duplicate sibling error")
	}
}

type teamRecordQueryRunner struct {
	rows *teamRecordRows
}

func (r *teamRecordQueryRunner) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, fmt.Errorf("unexpected exec")
}

func (r *teamRecordQueryRunner) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return r.rows, nil
}

func (r *teamRecordQueryRunner) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}

type teamRecordRow struct {
	ID                 int
	Name               string
	Kind               string
	ParentID           sql.NullInt32
	Description        sql.NullString
	RepoURL            string
	RepositoryFullName string
}

type teamRecordRows struct {
	rows []teamRecordRow
	idx  int
}

func (r *teamRecordRows) Close() {}

func (r *teamRecordRows) Err() error { return nil }

func (r *teamRecordRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func (r *teamRecordRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *teamRecordRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}

func (r *teamRecordRows) Scan(dest ...any) error {
	row := r.rows[r.idx-1]
	values := []any{row.ID, row.Name, row.Kind, row.ParentID, row.Description, row.RepoURL, row.RepositoryFullName}
	for idx, value := range values {
		switch target := dest[idx].(type) {
		case *int:
			*target = value.(int)
		case *string:
			*target = value.(string)
		case *sql.NullInt32:
			*target = value.(sql.NullInt32)
		case *sql.NullString:
			*target = value.(sql.NullString)
		default:
			return fmt.Errorf("unsupported scan destination %T", dest[idx])
		}
	}
	return nil
}

func (r *teamRecordRows) Values() ([]any, error) { return nil, nil }

func (r *teamRecordRows) RawValues() [][]byte { return nil }

func (r *teamRecordRows) Conn() *pgx.Conn { return nil }
