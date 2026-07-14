package nopsai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nopsai/pkg/models"
)

type fakeConfigSyncGitReader struct {
	accessErr error
	dirErrs   map[string]error
	fileErrs  map[string]error
	dirs      map[string]map[string]string
	files     map[string]string

	accessChecks   []string
	requestedDirs  []string
	requestedFiles []string
}

func (f *fakeConfigSyncGitReader) EnsureAccessible(_ context.Context) error {
	f.accessChecks = append(f.accessChecks, "access")
	return f.accessErr
}

func (f *fakeConfigSyncGitReader) Directory(_ context.Context, ref, path string) (map[string]string, error) {
	f.requestedDirs = append(f.requestedDirs, ref+":"+path)
	if err := f.dirErrs[path]; err != nil {
		return nil, err
	}
	if files, ok := f.dirs[path]; ok {
		copy := make(map[string]string, len(files))
		for key, value := range files {
			copy[key] = value
		}
		return copy, nil
	}
	return map[string]string{}, nil
}

func (f *fakeConfigSyncGitReader) File(_ context.Context, ref, path string, notFoundErr error) (string, error) {
	f.requestedFiles = append(f.requestedFiles, ref+":"+path)
	if err := f.fileErrs[path]; err != nil {
		return "", err
	}
	if content, ok := f.files[path]; ok {
		return content, nil
	}
	return "", notFoundErr
}

func TestNewConfigSyncRepositoryContextNormalizesBinding(t *testing.T) {
	ctx, err := newConfigSyncRepositoryContext(models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   " /team-1/platform/ ",
		RepoURL:   " https://github.com/acme/platform-config.git ",
		BasePath:  " config\\prod/ ",
	})
	if err != nil {
		t.Fatalf("newConfigSyncRepositoryContext() error = %v", err)
	}

	if ctx.owner != "acme" || ctx.repo != "platform-config" {
		t.Fatalf("repository = %s/%s, want acme/platform-config", ctx.owner, ctx.repo)
	}
	if ctx.provider != models.ConfigRepositoryProviderGitHub || ctx.project != "acme/platform-config" {
		t.Fatalf("provider/project = %s/%s, want github/acme/platform-config", ctx.provider, ctx.project)
	}
	if ctx.branch != "main" {
		t.Fatalf("branch = %q, want main", ctx.branch)
	}
	if ctx.basePath != "config/prod" {
		t.Fatalf("basePath = %q, want config/prod", ctx.basePath)
	}
	if ctx.boundTeam != "team-1/platform" {
		t.Fatalf("boundTeam = %q, want team-1/platform", ctx.boundTeam)
	}
	if ctx.dirs.pipeline != "config/prod/pipelines" || ctx.dirs.configRepository != "config/prod/config-repositories" {
		t.Fatalf("dirs = %#v, want base-path-qualified pipeline and config repository dirs", ctx.dirs)
	}
}

func TestNewConfigSyncRepositoryContextRequiresTeamScopeID(t *testing.T) {
	_, err := newConfigSyncRepositoryContext(models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		RepoURL:   "https://github.com/acme/platform-config",
	})
	if err == nil || !strings.Contains(err.Error(), "team-scoped config repository is missing its scope_id") {
		t.Fatalf("newConfigSyncRepositoryContext() error = %v, want missing scope_id error", err)
	}
}

func TestFetchConfigSyncRepositoryFilesAddsTeamNotificationRoot(t *testing.T) {
	reader := &fakeConfigSyncGitReader{
		files: map[string]string{
			"config/notifications.yaml": "routes: []\n",
			"config/ai-profiles.yaml":   "llm_profiles: []\n",
		},
	}
	binding := models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
		RepoURL:   "https://github.com/acme/platform-config",
		Branch:    "release",
		BasePath:  "config",
	}
	repoCtx, err := newConfigSyncRepositoryContext(binding)
	if err != nil {
		t.Fatalf("newConfigSyncRepositoryContext() error = %v", err)
	}

	files, err := fetchConfigSyncRepositoryFiles(context.Background(), reader, repoCtx, binding)
	if err != nil {
		t.Fatalf("fetchConfigSyncRepositoryFiles() error = %v", err)
	}

	if got := files.notifications["config/notifications.yaml"]; got != "routes: []\n" {
		t.Fatalf("root notification content = %q, want fetched route", got)
	}
	if got := files.teamAIProfiles["config/ai-profiles.yaml"]; got != "llm_profiles: []\n" {
		t.Fatalf("team AI profile content = %q, want fetched profile file", got)
	}
	if len(reader.accessChecks) != 1 || reader.accessChecks[0] != "access" {
		t.Fatalf("access checks = %#v, want one access check", reader.accessChecks)
	}
	wantRequestedFiles := []string{
		"release:config/notifications.yaml",
		"release:config/ai-profiles.yaml",
		"release:config/ai-profiles.yml",
	}
	if !sameStringSet(reader.requestedFiles, wantRequestedFiles) {
		t.Fatalf("requested files = %#v, want %#v", reader.requestedFiles, wantRequestedFiles)
	}
	if !containsString(reader.requestedDirs, "release:config/pipelines") {
		t.Fatalf("requested dirs = %#v, want pipeline directory request", reader.requestedDirs)
	}
}

func TestFetchConfigSyncRepositoryFilesIgnoresMissingTeamNotificationRoot(t *testing.T) {
	binding := models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
		RepoURL:   "https://github.com/acme/platform-config",
		BasePath:  "config",
	}
	repoCtx, err := newConfigSyncRepositoryContext(binding)
	if err != nil {
		t.Fatalf("newConfigSyncRepositoryContext() error = %v", err)
	}

	files, err := fetchConfigSyncRepositoryFiles(context.Background(), &fakeConfigSyncGitReader{}, repoCtx, binding)
	if err != nil {
		t.Fatalf("fetchConfigSyncRepositoryFiles() error = %v", err)
	}
	if _, ok := files.notifications["config/notifications.yaml"]; ok {
		t.Fatalf("root notification should not be added when git-bot reports not found")
	}
	if len(files.teamAIProfiles) != 0 {
		t.Fatalf("team AI profiles should not be added when git-bot reports not found")
	}
}

func TestFetchConfigSyncRepositoryFilesWrapsDirectoryErrors(t *testing.T) {
	dirErr := errors.New("git-bot unavailable")
	reader := &fakeConfigSyncGitReader{
		dirErrs: map[string]error{"cfg/steps": dirErr},
	}
	binding := models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
		RepoURL:   "https://github.com/acme/platform-config",
		BasePath:  "cfg",
	}
	repoCtx, err := newConfigSyncRepositoryContext(binding)
	if err != nil {
		t.Fatalf("newConfigSyncRepositoryContext() error = %v", err)
	}

	_, err = fetchConfigSyncRepositoryFiles(context.Background(), reader, repoCtx, binding)
	if !errors.Is(err, dirErr) || !strings.Contains(err.Error(), "failed to fetch reusable steps") {
		t.Fatalf("fetchConfigSyncRepositoryFiles() error = %v, want wrapped reusable steps error", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for _, value := range want {
		if !containsString(got, value) {
			return false
		}
	}
	return true
}
