package nopsai

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestNormalizeTaskOutputResolveNamesValidatesAndDeduplicates(t *testing.T) {
	names, ok := normalizeTaskOutputResolveNames([]string{" image_tag ", "IMAGE_TAG", "image_tag"})
	if !ok {
		t.Fatal("normalizeTaskOutputResolveNames() ok = false, want true")
	}
	if len(names) != 2 || names[0] != "image_tag" || names[1] != "IMAGE_TAG" {
		t.Fatalf("names = %#v, want deduplicated ordered input names", names)
	}

	if _, ok := normalizeTaskOutputResolveNames([]string{"image-tag"}); ok {
		t.Fatal("normalizeTaskOutputResolveNames() ok = true, want invalid output name")
	}
}

func TestResolveChildTaskOutputsReturnsRequestedOutputsInOrder(t *testing.T) {
	db := &fakeChildOutputDB{
		childMatches: true,
		rows: []fakeChildOutputRow{
			{stepName: "build", taskName: "publish", name: "access_token", value: "encrypted-token", sensitive: true, sizeBytes: 15},
			{stepName: "build", taskName: "publish", name: "image_tag", value: "v1.2.3", sizeBytes: 6},
		},
	}
	resolution, err := resolveChildTaskOutputs(context.Background(), db, func(value string) (string, error) {
		if value != "encrypted-token" {
			t.Fatalf("decrypt value = %q, want encrypted-token", value)
		}
		return "plain-token", nil
	}, "child-run", "parent-run", "child-build", []string{"image_tag", "access_token"})
	if err != nil {
		t.Fatalf("resolveChildTaskOutputs() error = %v", err)
	}
	if len(resolution.response.Outputs) != 2 {
		t.Fatalf("outputs = %#v, want 2", resolution.response.Outputs)
	}
	if got := resolution.response.Outputs[0]; got.Name != "image_tag" || got.Value != "v1.2.3" || got.Sensitive {
		t.Fatalf("first output = %#v, want image_tag", got)
	}
	if got := resolution.response.Outputs[1]; got.Name != "access_token" || got.Value != "plain-token" || !got.Sensitive || got.SizeBytes != 15 {
		t.Fatalf("second output = %#v, want decrypted access_token", got)
	}
	if len(resolution.auditOutputs) != 2 {
		t.Fatalf("audit outputs = %#v, want metadata for both outputs", resolution.auditOutputs)
	}
}

func TestResolveChildTaskOutputsRejectsInvalidLineage(t *testing.T) {
	_, err := resolveChildTaskOutputs(context.Background(), &fakeChildOutputDB{}, nil, "child-run", "parent-run", "child-build", []string{"image_tag"})
	if !errors.Is(err, errChildOutputLineage) {
		t.Fatalf("resolveChildTaskOutputs() error = %v, want lineage error", err)
	}
}

func TestResolveChildTaskOutputsRejectsAmbiguousNames(t *testing.T) {
	db := &fakeChildOutputDB{
		childMatches: true,
		rows: []fakeChildOutputRow{
			{stepName: "build", taskName: "publish", name: "image_tag", value: "v1.2.3"},
			{stepName: "release", taskName: "publish", name: "image_tag", value: "v1.2.4"},
		},
	}
	_, err := resolveChildTaskOutputs(context.Background(), db, nil, "child-run", "parent-run", "child-build", []string{"image_tag"})
	if !errors.Is(err, errChildOutputAmbiguous) {
		t.Fatalf("resolveChildTaskOutputs() error = %v, want ambiguous error", err)
	}
}

func TestResolveChildTaskOutputsRejectsMissingRequestedOutput(t *testing.T) {
	db := &fakeChildOutputDB{
		childMatches: true,
		rows: []fakeChildOutputRow{
			{stepName: "build", taskName: "publish", name: "image_tag", value: "v1.2.3"},
		},
	}
	_, err := resolveChildTaskOutputs(context.Background(), db, nil, "child-run", "parent-run", "child-build", []string{"image_tag", "digest"})
	if !errors.Is(err, errChildOutputMissing) {
		t.Fatalf("resolveChildTaskOutputs() error = %v, want missing output error", err)
	}
}

type fakeChildOutputDB struct {
	childMatches bool
	rows         []fakeChildOutputRow
}

func (db *fakeChildOutputDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return fakeChildOutputExistsRow{exists: db.childMatches}
}

func (db *fakeChildOutputDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return &fakeChildOutputRows{rows: db.rows, idx: -1}, nil
}

type fakeChildOutputExistsRow struct {
	exists bool
}

func (row fakeChildOutputExistsRow) Scan(dest ...any) error {
	if len(dest) != 1 {
		return errors.New("expected one destination")
	}
	value, ok := dest[0].(*bool)
	if !ok {
		return errors.New("expected bool destination")
	}
	*value = row.exists
	return nil
}

type fakeChildOutputRow struct {
	stepName  string
	taskName  string
	name      string
	value     string
	sensitive bool
	sizeBytes int64
}

type fakeChildOutputRows struct {
	rows []fakeChildOutputRow
	idx  int
}

func (rows *fakeChildOutputRows) Close() {}

func (rows *fakeChildOutputRows) Err() error { return nil }

func (rows *fakeChildOutputRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func (rows *fakeChildOutputRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (rows *fakeChildOutputRows) Next() bool {
	rows.idx++
	return rows.idx < len(rows.rows)
}

func (rows *fakeChildOutputRows) Scan(dest ...any) error {
	if rows.idx < 0 || rows.idx >= len(rows.rows) {
		return errors.New("scan called without current row")
	}
	if len(dest) != 6 {
		return errors.New("expected six destinations")
	}
	row := rows.rows[rows.idx]
	values := []any{row.stepName, row.taskName, row.name, row.value, row.sensitive, row.sizeBytes}
	for idx, value := range values {
		switch dest := dest[idx].(type) {
		case *string:
			text, ok := value.(string)
			if !ok {
				return errors.New("expected string value")
			}
			*dest = text
		case *bool:
			boolean, ok := value.(bool)
			if !ok {
				return errors.New("expected bool value")
			}
			*dest = boolean
		case *int64:
			number, ok := value.(int64)
			if !ok {
				return errors.New("expected int64 value")
			}
			*dest = number
		default:
			return errors.New("unexpected scan destination")
		}
	}
	return nil
}

func (rows *fakeChildOutputRows) Values() ([]any, error) { return nil, nil }

func (rows *fakeChildOutputRows) RawValues() [][]byte { return nil }

func (rows *fakeChildOutputRows) Conn() *pgx.Conn { return nil }
