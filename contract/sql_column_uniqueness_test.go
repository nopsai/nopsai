package contract

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// TestSQLStatementsDoNotRepeatColumns guards against a rename reaching a SQL
// identifier it should not have. Commit 18c0d183 renamed the "llm_profile"
// concept to "model" and the replacement landed inside the ai_usage_events
// INSERT and CREATE TABLE, producing `feature, provider, model, model`.
// PostgreSQL rejects that with 42701, so every AI usage event silently failed
// to record for six days. The existing schema tests only string-matched
// statements and could not see it.
//
// This walks every SQL literal in the tree rather than the one table that broke,
// because the next botched rename will not be in the same file.
func TestSQLStatementsDoNotRepeatColumns(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to resolve repository root: %v", err)
	}
	checked := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipSQLScanDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !sqlScanTarget(path) {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			relative = path
		}
		for _, statement := range sqlColumnLists(string(contents)) {
			checked++
			if duplicate, ok := firstRepeatedColumn(statement.columns); ok {
				t.Errorf(
					"%s: %s %s names column %q more than once; PostgreSQL rejects this with 42701",
					relative, statement.kind, statement.table, duplicate,
				)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to scan repository: %v", err)
	}
	// A refactor that moves every statement out of Go literals would otherwise
	// turn this test green by scanning nothing.
	if checked == 0 {
		t.Fatal("scanned no SQL column lists; the scanner is no longer finding statements")
	}
}

type sqlColumnList struct {
	kind    string
	table   string
	columns []string
}

// sqlColumnLists extracts the column list from every INSERT INTO and CREATE
// TABLE in the given source. It is deliberately narrow: it only understands the
// two statement shapes that can carry a duplicate column, and it ignores
// anything it cannot parse confidently.
func sqlColumnLists(source string) []sqlColumnList {
	lists := []sqlColumnList{}
	lower := strings.ToLower(source)
	for _, prefix := range []struct {
		keyword string
		kind    string
	}{
		{"insert into", "INSERT INTO"},
		{"create table", "CREATE TABLE"},
	} {
		search := 0
		for {
			index := strings.Index(lower[search:], prefix.keyword)
			if index < 0 {
				break
			}
			start := search + index
			search = start + len(prefix.keyword)

			table, open, ok := sqlTableAndOpenParen(source, search)
			if !ok {
				continue
			}
			body, ok := sqlBalancedParen(source, open)
			if !ok {
				continue
			}
			columns := sqlColumnNames(body, prefix.kind)
			if len(columns) == 0 {
				continue
			}
			lists = append(lists, sqlColumnList{kind: prefix.kind, table: table, columns: columns})
		}
	}
	return lists
}

// sqlTableAndOpenParen reads the table name that follows the statement keyword
// and returns the index of the opening parenthesis of its column list. Anything
// between the two that is not an identifier — "IF NOT EXISTS", whitespace — is
// skipped. A statement with no parenthesised column list (INSERT ... SELECT) is
// reported as not found.
func sqlTableAndOpenParen(source string, from int) (string, int, bool) {
	table := ""
	for index := from; index < len(source); index++ {
		char := rune(source[index])
		switch {
		case char == '(':
			if table == "" {
				return "", 0, false
			}
			return table, index, true
		case unicode.IsSpace(char):
			continue
		case isSQLIdentifierRune(char):
			word := readSQLWord(source, index)
			index += len(word) - 1
			if !isSQLNoiseWord(word) {
				table = word
			}
		default:
			// A quote, semicolon or template placeholder means this is not a
			// shape we can read; give up on this occurrence.
			return "", 0, false
		}
	}
	return "", 0, false
}

// sqlBalancedParen returns the contents between the parenthesis at open and its
// matching close.
func sqlBalancedParen(source string, open int) (string, bool) {
	depth := 0
	for index := open; index < len(source); index++ {
		switch source[index] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return source[open+1 : index], true
			}
		}
	}
	return "", false
}

// sqlColumnNames splits a column list on top-level commas and returns the
// column name of each entry. Nested parentheses — NUMERIC(18, 8),
// REFERENCES teams(id) — do not split.
func sqlColumnNames(body string, kind string) []string {
	names := []string{}
	depth := 0
	current := strings.Builder{}
	flush := func() {
		entry := strings.TrimSpace(current.String())
		current.Reset()
		if entry == "" {
			return
		}
		name := readSQLWord(entry, 0)
		if name == "" {
			return
		}
		if kind == "CREATE TABLE" && isSQLTableConstraintWord(name) {
			return
		}
		names = append(names, strings.ToLower(name))
	}
	for _, char := range body {
		switch {
		case char == '(':
			depth++
			current.WriteRune(char)
		case char == ')':
			depth--
			current.WriteRune(char)
		case char == ',' && depth == 0:
			flush()
		default:
			current.WriteRune(char)
		}
	}
	flush()

	// An INSERT column list is bare identifiers. Anything else — a VALUES tuple
	// caught by a malformed match, an expression — is not a column list, and
	// guessing at it would produce noise instead of findings.
	if kind == "INSERT INTO" {
		for _, name := range names {
			if !isSQLBareIdentifier(name) {
				return nil
			}
		}
	}
	return names
}

func firstRepeatedColumn(columns []string) (string, bool) {
	seen := map[string]bool{}
	for _, column := range columns {
		if seen[column] {
			return column, true
		}
		seen[column] = true
	}
	return "", false
}

func readSQLWord(source string, from int) string {
	end := from
	for end < len(source) && isSQLIdentifierRune(rune(source[end])) {
		end++
	}
	return source[from:end]
}

func isSQLIdentifierRune(char rune) bool {
	return char == '_' || char == '.' || unicode.IsLetter(char) || unicode.IsDigit(char)
}

func isSQLBareIdentifier(word string) bool {
	if word == "" {
		return false
	}
	for _, char := range word {
		if !isSQLIdentifierRune(char) {
			return false
		}
	}
	return true
}

func isSQLNoiseWord(word string) bool {
	switch strings.ToLower(word) {
	case "if", "not", "exists", "unlogged", "temp", "temporary":
		return true
	default:
		return false
	}
}

func isSQLTableConstraintWord(word string) bool {
	switch strings.ToLower(word) {
	case "primary", "unique", "foreign", "check", "constraint", "exclude", "like":
		return true
	default:
		return false
	}
}

func skipSQLScanDirectory(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", "build", ".next":
		return true
	default:
		return false
	}
}

func sqlScanTarget(path string) bool {
	switch filepath.Ext(path) {
	case ".sql":
		return true
	case ".go":
		// Test sources carry deliberately malformed fixtures.
		return !strings.HasSuffix(path, "_test.go")
	default:
		return false
	}
}
