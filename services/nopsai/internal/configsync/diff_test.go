package configsync

import "testing"

func TestDiffFilesReportsSortedStatuses(t *testing.T) {
	items := DiffFiles(
		map[string]string{
			"b/modified.yaml":  "old",
			"c/deleted.yaml":   "removed",
			"d/unchanged.yaml": "same",
		},
		map[string]string{
			"a/added.yaml":     "new",
			"b/modified.yaml":  "updated",
			"d/unchanged.yaml": "same",
		},
		nil,
	)

	want := []struct {
		path   string
		status string
		del    bool
	}{
		{"a/added.yaml", "added", false},
		{"b/modified.yaml", "modified", false},
		{"c/deleted.yaml", "deleted", true},
		{"d/unchanged.yaml", "unchanged", false},
	}
	if len(items) != len(want) {
		t.Fatalf("DiffFiles() returned %d items, want %d", len(items), len(want))
	}
	for i, expected := range want {
		if items[i].Path != expected.path || items[i].Status != expected.status || items[i].Delete != expected.del {
			t.Fatalf("item %d = %+v, want path %q status %q delete %t", i, items[i], expected.path, expected.status, expected.del)
		}
	}
	if items[0].GitContent != nil || items[0].DesiredContent == nil || *items[0].DesiredContent != "new" {
		t.Fatalf("added item content = git %v desired %v, want only desired content", items[0].GitContent, items[0].DesiredContent)
	}
	if items[2].GitContent == nil || *items[2].GitContent != "removed" || items[2].DesiredContent != nil {
		t.Fatalf("deleted item content = git %v desired %v, want only git content", items[2].GitContent, items[2].DesiredContent)
	}
}

func TestDiffFilesUsesCustomEquality(t *testing.T) {
	items := DiffFiles(
		map[string]string{"knowledge/service.md": "name: API\nbody"},
		map[string]string{"knowledge/service.md": "body"},
		func(filePath, gitContent, desiredContent string) bool {
			return filePath == "knowledge/service.md" && gitContent == "name: API\nbody" && desiredContent == "body"
		},
	)

	if len(items) != 1 {
		t.Fatalf("DiffFiles() returned %d items, want 1", len(items))
	}
	if items[0].Status != "unchanged" {
		t.Fatalf("custom equality item status = %q, want unchanged", items[0].Status)
	}
}

func TestNormalizeFileContent(t *testing.T) {
	got := NormalizeFileContent("a\r\nb\r\n\r\n")
	if got != "a\nb\n" {
		t.Fatalf("NormalizeFileContent() = %q, want %q", got, "a\nb\n")
	}

	got = NormalizeFileContent("a")
	if got != "a\n" {
		t.Fatalf("NormalizeFileContent() = %q, want trailing newline", got)
	}
}
