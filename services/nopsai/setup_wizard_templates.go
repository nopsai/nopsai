package nopsai

import (
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/services/nopsai/internal/configsync"

	"gopkg.in/yaml.v3"
)

func setupProfiles() []setupStarterProfile {
	return []setupStarterProfile{
		{ID: setupProfileDev, Label: "Dev", Description: "Local evaluation with starter teams, a smoke pipeline, and generated local values."},
		{ID: setupProfileTeam, Label: "Team", Description: "Shared workspace defaults for owners, developers, viewers, repository triggers, and GitOps handoff."},
		{ID: setupProfileProduction, Label: "Production", Description: "GitOps-first setup with stricter guardrails and no direct database starter seed."},
		{ID: setupProfileEmpty, Label: "Empty", Description: "Only validate prerequisites and leave resources for manual configuration."},
	}
}

func normalizeSetupProfile(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", setupProfileDev:
		return setupProfileDev
	case setupProfileTeam:
		return setupProfileTeam
	case "prod", setupProfileProduction:
		return setupProfileProduction
	case setupProfileEmpty, "blank":
		return setupProfileEmpty
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func setupGitBotURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func setupRuntimeEnvironment() string {
	for _, key := range []string{"NOPSAI_ENV", "APP_ENV", "ENVIRONMENT", "GO_ENV"} {
		if value := strings.TrimSpace(strings.ToLower(os.Getenv(key))); value != "" {
			return value
		}
	}
	return "development"
}

func setupRepositoriesFromQuery(values url.Values) []string {
	var repositories []string
	repositories = append(repositories, values["repository"]...)
	for _, raw := range values["repositories"] {
		repositories = append(repositories, strings.Split(raw, ",")...)
	}
	return normalizeSetupRepositories(repositories)
}

func setupTemplateOptionsFromQuery(values url.Values) setupTemplateOptions {
	includeLLM := queryBoolDefault(values, "include_llm", true)
	includeMCP := queryBoolDefault(values, "mcp_examples", true)
	return setupTemplateOptions{
		RepositoryTeams: setupRepositoryTeamsFromQuery(values),
		Users:           setupUsersFromQuery(values),
		IncludeLLM:      includeLLM,
		IncludeMCP:      includeMCP,
		LLMProfile: setupLLMProfileInput{
			Name:          "standard",
			Provider:      values.Get("llm_provider"),
			Model:         values.Get("llm_model"),
			BaseURL:       values.Get("llm_base_url"),
			CredentialRef: values.Get("llm_credential_ref"),
			AllowedScopes: []string{
				"dev",
				"prod",
			},
		},
	}
}

func queryBoolDefault(values url.Values, key string, defaultValue bool) bool {
	raw := strings.TrimSpace(strings.ToLower(values.Get(key)))
	if raw == "" {
		return defaultValue
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func setupRepositoryTeamsFromQuery(values url.Values) []setupRepositoryTeamInput {
	var teams []setupRepositoryTeamInput
	rawTeams := append([]string{}, values["repository_team"]...)
	for _, raw := range rawTeams {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		name, reposText, found := strings.Cut(raw, ":")
		if !found {
			name, reposText, found = strings.Cut(raw, "=")
		}
		if !found {
			continue
		}
		teams = append(teams, setupRepositoryTeamInput{
			Name:         name,
			Repositories: strings.Split(reposText, ","),
		})
	}
	return normalizeSetupRepositoryTeams(teams, nil)
}

func setupUsersFromQuery(values url.Values) []setupUserInput {
	var users []setupUserInput
	for _, raw := range values["setup_user"] {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		var user setupUserInput
		if err := json.Unmarshal([]byte(raw), &user); err != nil {
			continue
		}
		users = append(users, user)
	}
	return normalizeSetupUsers(users)
}

func normalizeSetupUsers(raw []setupUserInput) []setupUserInput {
	users := make([]setupUserInput, 0, len(raw))
	for _, user := range raw {
		user.Sub = strings.TrimSpace(user.Sub)
		user.Email = strings.TrimSpace(user.Email)
		user.Role = strings.TrimSpace(user.Role)
		user.Team = normalizeSetupRepositoryTeamName(user.Team)
		if user.Sub == "" {
			user.Sub = user.Email
		}
		if user.Email == "" {
			user.Email = user.Sub
		}
		if user.Sub == "" {
			continue
		}
		users = append(users, user)
	}
	return users
}

func normalizeSetupRepositories(raw []string) []string {
	seen := map[string]bool{}
	var repositories []string
	for _, value := range raw {
		repo := strings.Trim(strings.TrimSpace(value), "/")
		if repo == "" {
			continue
		}
		repo = strings.ReplaceAll(repo, "\\", "/")
		repo = strings.TrimPrefix(repo, "git@github.com:")
		repo = strings.TrimPrefix(repo, "https://github.com/")
		repo = strings.TrimPrefix(repo, "http://github.com/")
		repo = strings.TrimPrefix(repo, "github.com/")
		repo = strings.TrimSuffix(repo, ".git")
		parts := strings.Split(repo, "/")
		if len(parts) < 2 {
			continue
		}
		repo = strings.Trim(parts[0], "/") + "/" + strings.Trim(parts[1], "/")
		if strings.Contains(repo, "..") || seen[repo] {
			continue
		}
		seen[repo] = true
		repositories = append(repositories, repo)
	}
	sort.Strings(repositories)
	return repositories
}

func normalizeSetupRepositoryTeams(raw []setupRepositoryTeamInput, legacyRepositories []string) []setupRepositoryTeamInput {
	if len(raw) == 0 && len(legacyRepositories) > 0 {
		raw = []setupRepositoryTeamInput{{
			Name:         "applications",
			Repositories: legacyRepositories,
		}}
	}

	teamsByName := map[string][]string{}
	var teamOrder []string
	for _, team := range raw {
		name := normalizeSetupRepositoryTeamName(team.Name)
		if name == "" {
			continue
		}
		if _, exists := teamsByName[name]; !exists {
			teamOrder = append(teamOrder, name)
		}
		teamsByName[name] = append(teamsByName[name], normalizeSetupRepositories(team.Repositories)...)
	}
	if len(teamsByName) == 0 {
		return nil
	}

	result := make([]setupRepositoryTeamInput, 0, len(teamOrder))
	seenRepositories := map[string]bool{}
	for _, name := range teamOrder {
		repositories := normalizeSetupRepositories(teamsByName[name])
		filtered := make([]string, 0, len(repositories))
		for _, repo := range repositories {
			key := strings.ToLower(repo)
			if seenRepositories[key] {
				continue
			}
			seenRepositories[key] = true
			filtered = append(filtered, repo)
		}
		result = append(result, setupRepositoryTeamInput{Name: name, Repositories: filtered})
	}
	return result
}

func normalizeSetupRepositoryTeamName(raw string) string {
	name := strings.Trim(strings.TrimSpace(raw), "/")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.Join(strings.Fields(name), "-")
	if name == "" || strings.Contains(name, "..") {
		return ""
	}
	return name
}

func setupRepositoriesFromTeams(teams []setupRepositoryTeamInput) []string {
	var repositories []string
	for _, team := range teams {
		repositories = append(repositories, team.Repositories...)
	}
	return normalizeSetupRepositories(repositories)
}

func setupStarterTemplates(profile string, repositories []string) map[string]string {
	return setupStarterTemplatesWithOptions(profile, repositories, setupTemplateOptions{
		IncludeLLM: true,
		IncludeMCP: true,
		LLMProfile: setupLLMProfileInput{
			Name:          "standard",
			Provider:      config.LLMProviderLMStudio,
			Model:         "qwen3-coder",
			BaseURL:       "http://lmstudio:1234",
			AllowedScopes: []string{"dev", "prod"},
		},
	})
}

func setupStarterTemplatesWithOptions(profile string, repositories []string, options setupTemplateOptions) map[string]string {
	profile = normalizeSetupProfile(profile)
	repositoryTeams := normalizeSetupRepositoryTeams(options.RepositoryTeams, repositories)
	if len(repositoryTeams) > 0 {
		repositories = setupRepositoriesFromTeams(repositoryTeams)
	} else {
		repositories = normalizeSetupRepositories(repositories)
		repositoryTeams = normalizeSetupRepositoryTeams(nil, repositories)
	}
	files := map[string]string{
		"README.md":                      setupReadme(repositoryTeams, repositories),
		"pipelines/setup/first-run.yaml": setupFirstRunPipelineYAML(profile, options.IncludeLLM),
		"steps/setup/announce.yaml":      setupReusableStepYAML(),
		"scopes/dev/scope.yaml":          setupScopeYAML("dev", repositoryTeams, repositories),
		"access/bootstrap.yaml":          setupAccessYAML(repositoryTeams, options.Users),
	}
	if knowledgeTeam := setupStarterKnowledgeTeam(repositoryTeams, repositories); knowledgeTeam != "" {
		files["knowledge/guideline/"+knowledgeTeam+"/setup-run.md"] = setupKnowledgeMarkdown(repositoryTeams, repositories)
	}
	for path, content := range setupConfigRepositoryStructureFiles(repositoryTeams, repositories) {
		files[path] = content
	}
	if options.IncludeLLM {
		files["setting/system/llm_profile.yaml"] = setupLLMProfileYAML(options.LLMProfile)
	}
	if options.IncludeMCP {
		files["setting/system/mcp.yaml"] = setupMCPYAML()
	}
	if profile == setupProfileTeam || profile == setupProfileProduction {
		files["scopes/prod/scope.yaml"] = setupScopeYAML("prod", repositoryTeams, repositories)
	}
	for _, repo := range repositories {
		files["triggers/"+repo+".yaml"] = setupTriggerYAML(profile)
	}
	return files
}

func setupPipelineRunStructure(repositoryTeams []setupRepositoryTeamInput, repositories []string) map[string]*configsync.PipelineRunStructureNode {
	repositoryTeams = normalizeSetupRepositoryTeams(repositoryTeams, repositories)
	if len(repositoryTeams) == 0 {
		return map[string]*configsync.PipelineRunStructureNode{}
	}

	structure := make(map[string]*configsync.PipelineRunStructureNode, len(repositoryTeams))
	for _, team := range repositoryTeams {
		structure[team.Name] = setupPipelineRunStructureTeamNode(team)
	}
	return structure
}

func setupPipelineRunStructureTeamNode(team setupRepositoryTeamInput) *configsync.PipelineRunStructureNode {
	node := &configsync.PipelineRunStructureNode{
		Description: "Repository team " + team.Name,
		Children:    map[string]*configsync.PipelineRunStructureNode{},
	}
	for _, repo := range team.Repositories {
		parts := strings.Split(repo, "/")
		if len(parts) != 2 {
			continue
		}
		node.Apps = append(node.Apps, configsync.PipelineRunStructureApp{
			Name:               configsync.RepositoryDisplayNameFromFullName(repo),
			RepoURL:            configsync.CanonicalRepositoryURL(repo),
			RepositoryFullName: repo,
		})
		node.Repos = append(node.Repos, repo)
	}
	return node
}

func setupAccessTeam(profile string) string {
	switch normalizeSetupProfile(profile) {
	case setupProfileTeam:
		return "workspace"
	case setupProfileProduction:
		return "production"
	default:
		return "sandbox"
	}
}

func setupStarterTeamNames(repositoryTeams []setupRepositoryTeamInput, repositories []string) []string {
	return setupAccessGrantTeams(normalizeSetupRepositoryTeams(repositoryTeams, repositories))
}

func setupStarterKnowledgeTeam(repositoryTeams []setupRepositoryTeamInput, repositories []string) string {
	teams := setupStarterTeamNames(repositoryTeams, repositories)
	if len(teams) == 0 {
		return ""
	}
	return teams[0]
}

func setupStarterTeamValue(repositoryTeams []setupRepositoryTeamInput, repositories []string) string {
	return strings.Join(setupStarterTeamNames(repositoryTeams, repositories), ",")
}

func setupStarterTeamLabel(repositoryTeams []setupRepositoryTeamInput, repositories []string) string {
	teams := setupStarterTeamNames(repositoryTeams, repositories)
	quoted := make([]string, 0, len(teams))
	for _, team := range teams {
		quoted = append(quoted, fmt.Sprintf("`%s`", team))
	}
	if len(quoted) == 0 {
		return "teams: none"
	}
	if len(quoted) == 1 {
		return "team " + quoted[0]
	}
	return "teams " + strings.Join(quoted, ", ")
}

func setupScopeVariables(profile string, repositoryTeams []setupRepositoryTeamInput, repositories []string) map[string]map[string]string {
	workspace := setupStarterTeamValue(repositoryTeams, repositories)
	values := map[string]map[string]string{
		"dev": {
			"NOPSAI_SETUP_WORKSPACE": workspace,
			"NOPSAI_SETUP_SCOPE":     "dev",
		},
	}
	if profile == setupProfileTeam || profile == setupProfileProduction {
		values["prod"] = map[string]string{
			"NOPSAI_SETUP_WORKSPACE": workspace,
			"NOPSAI_SETUP_SCOPE":     "prod",
		}
	}
	return values
}

type setupKnowledgeContextSeed struct {
	kind        string
	team        string
	name        string
	description string
	content     string
}

func setupKnowledgeContexts(repositoryTeams []setupRepositoryTeamInput, repositories []string) []setupKnowledgeContextSeed {
	team := setupStarterKnowledgeTeam(repositoryTeams, repositories)
	if team == "" {
		return nil
	}
	return []setupKnowledgeContextSeed{{
		kind:        "guideline",
		team:        team,
		name:        "setup-run",
		description: "Starter pipeline run expectations",
		content:     fmt.Sprintf("Use the starter pipeline for %s to verify runner connectivity, log streaming, and optional LLM execution before attaching production repositories.", setupStarterTeamLabel(repositoryTeams, repositories)),
	}}
}

func setupReusableStepYAML() string {
	return strings.TrimSpace(`
name: announce
image: alpine:3.20
script: |
  #!/bin/sh
  set -e
  echo "Starting NopsAI setup pipeline"
`) + "\n"
}

func setupFirstRunPipelineYAML(profile string, includeLLM bool) string {
	scope := "dev"
	if normalizeSetupProfile(profile) == setupProfileProduction {
		scope = "prod"
	}
	description := "Verifies that NopsAI can run a starter job and stream logs without requiring an LLM profile."
	llmSettings := "llm_enabled: false"
	aiSmokeStep := ""
	if includeLLM {
		description = "Verifies that NopsAI can run a starter job, stream logs, and call the configured LLM profile."
		llmSettings = "llm_profile: standard"
		aiSmokeStep = `

  - name: ai-smoke
    goal: Return one short sentence confirming that the NopsAI setup smoke test reached the agent.
    ignore_failure: true
    depends_on:
      - runner-smoke`
	}
	return strings.TrimSpace(fmt.Sprintf(`
name: first-run
version: "1.0.0"
description: %s
container_image: alpine:3.20
working_directory: /workspace
timeout: 10m
%s
display_options:
  github_view: flat
variables:
  - %s:NOPSAI_SETUP_WORKSPACE
  - %s:NOPSAI_SETUP_SCOPE
steps:
  - name: announce
    include: step:setup/announce

  - name: runner-smoke
    image: alpine:3.20
    script: |
      #!/bin/sh
      set -e
      echo "NopsAI runner is executing the setup smoke test"
      echo "workspace=$NOPSAI_SETUP_WORKSPACE scope=$NOPSAI_SETUP_SCOPE"
    depends_on:
      - announce%s
`, description, llmSettings, scope, scope, aiSmokeStep)) + "\n"
}

func setupTriggerYAML(profile string) string {
	scope := "dev"
	if normalizeSetupProfile(profile) == setupProfileProduction {
		scope = "prod"
	}
	return strings.TrimSpace(fmt.Sprintf(`
triggers:
  - on: push
    branches:
      - main
    scope: %s
    pipelines:
      - setup/first-run

  - on: pull_request
    scope: %s
    pipelines:
      - setup/first-run
`, scope, scope)) + "\n"
}

func setupScopeYAML(scope string, repositoryTeams []setupRepositoryTeamInput, repositories []string) string {
	return strings.TrimSpace(fmt.Sprintf(`
variables:
  NOPSAI_SETUP_WORKSPACE: %q
  NOPSAI_SETUP_SCOPE: %s
secrets:
  GEMINI_API_KEY:
`, setupStarterTeamValue(repositoryTeams, repositories), scope)) + "\n"
}

func setupReadme(repositoryTeams []setupRepositoryTeamInput, repositories []string) string {
	return fmt.Sprintf("# NopsAI starter config\n\nStarter %s\n\nThis repository contains starter resources for the first NopsAI bootstrap. Keep plaintext secrets outside this repository. Scope files define plaintext scoped values under `variables:` and may define secret keys under `secrets:` with `null` placeholders or encrypted values generated by this NopsAI instance.\n", setupStarterTeamLabel(repositoryTeams, repositories))
}

func setupKnowledgeMarkdown(repositoryTeams []setupRepositoryTeamInput, repositories []string) string {
	return fmt.Sprintf("---\ndescription: Starter setup run expectations\n---\n\nUse the starter pipeline for %s to prove runner connectivity, logs, repository triggers, and optional LLM execution before onboarding production automation.\n", setupStarterTeamLabel(repositoryTeams, repositories))
}

func setupAccessYAML(repositoryTeams []setupRepositoryTeamInput, users []setupUserInput) string {
	teams := setupAccessGrantTeams(repositoryTeams)
	var builder strings.Builder
	if len(users) == 0 {
		builder.WriteString("users: []\n")
	} else {
		builder.WriteString("users:\n")
		for _, user := range users {
			sub := strings.TrimSpace(firstNonEmptyString(user.Sub, user.Email))
			if sub == "" {
				continue
			}
			email := strings.TrimSpace(firstNonEmptyString(user.Email, sub))
			fmt.Fprintf(&builder, "  - sub: %q\n", sub)
			fmt.Fprintf(&builder, "    email: %q\n", email)
			builder.WriteString("    provider: local\n")
			builder.WriteString("    status: active\n")
			builder.WriteString("    # password is intentionally not generated into GitOps; set it out of band if GitOps creates the account.\n")
		}
	}
	builder.WriteString("\nbasic_roles:\n")
	for _, team := range teams {
		builder.WriteString("  - user: admin\n")
		builder.WriteString("    role: owner\n")
		fmt.Fprintf(&builder, "    resource: team:%s\n", team)
	}
	for _, user := range users {
		sub := strings.TrimSpace(firstNonEmptyString(user.Sub, user.Email))
		if sub == "" {
			continue
		}
		role := strings.TrimSpace(user.Role)
		if role == "" {
			role = productRoleViewer
		}
		if normalizedRole, err := normalizeProductRoleName(role); err == nil {
			role = normalizedRole
		}
		team := setupAccessUserTeam(teams, user.Team)
		if team == "" {
			continue
		}
		fmt.Fprintf(&builder, "  - user: %q\n", sub)
		fmt.Fprintf(&builder, "    role: %s\n", role)
		fmt.Fprintf(&builder, "    resource: team:%s\n", team)
	}
	return builder.String()
}

func setupAccessGrantTeams(repositoryTeams []setupRepositoryTeamInput) []string {
	seen := map[string]bool{}
	var teams []string
	for _, team := range repositoryTeams {
		name := normalizeSetupRepositoryTeamName(team.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		teams = append(teams, name)
	}
	return teams
}

func setupAccessUserTeam(teams []string, raw string) string {
	if len(teams) == 0 {
		return ""
	}
	target := normalizeSetupRepositoryTeamName(raw)
	if target != "" {
		for _, team := range teams {
			if strings.EqualFold(team, target) {
				return team
			}
		}
	}
	return teams[0]
}

func setupLLMProfileYAML(input setupLLMProfileInput) string {
	provider := config.NormalizeLLMProvider(input.Provider)
	if provider == "" {
		provider = config.LLMProviderLMStudio
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = config.DefaultLLMProviderModel(provider)
	}
	credentialRef := strings.TrimSpace(input.CredentialRef)
	if credentialRef == "" && config.LLMProviderRequiresAPIKey(provider) {
		credentialRef = "credential://system/llm/" + credentialReferenceSegment(firstNonEmptyString(input.Name, config.DefaultLLMProfileName))
	}
	baseURL := strings.TrimSpace(input.BaseURL)
	if baseURL == "" {
		baseURL = config.DefaultLLMProviderBaseURL(provider)
	}

	document := struct {
		DefaultProfile string `yaml:"default_profile"`
		Profiles       []struct {
			Name              string `yaml:"name"`
			config.LLMProfile `yaml:",inline"`
		} `yaml:"profiles"`
	}{
		DefaultProfile: config.DefaultLLMProfileName,
	}
	document.Profiles = append(document.Profiles, struct {
		Name              string `yaml:"name"`
		config.LLMProfile `yaml:",inline"`
	}{
		Name: config.DefaultLLMProfileName,
		LLMProfile: config.NormalizeLLMProfile(config.LLMProfile{
			Provider:       provider,
			Model:          model,
			BaseURL:        baseURL,
			CredentialRef:  credentialRef,
			AllowedScopes:  []string{"dev", "prod"},
			TimeoutSeconds: input.TimeoutSeconds,
			MaxTokens:      input.MaxTokens,
			Temperature:    input.Temperature,
			Extra:          input.Extra,
		}),
	})
	contents, err := yaml.Marshal(document)
	if err != nil {
		return ""
	}
	return string(contents)
}

func setupMCPYAML() string {
	return strings.TrimSpace(`
mcp_servers:
  github-readonly:
    display_name: GitHub MCP Read-only
    enabled: false
    provider: github
    transport: streamable_http
    url: https://api.githubcopilot.com/mcp/x/all/readonly
    auth_type: bearer_token
    credential_ref: credential://system/mcp/github-readonly
    timeout: 30s
    allowed_scopes: ["dev", "prod"]

mcp_profiles:
  github-readonly:
    description: Read-only GitHub tools for repository-aware setup tests
    enabled: false
    servers:
      - server: github-readonly
        tools:
          - "*"
    allowed_scopes: ["dev", "prod"]
`) + "\n"
}

func setupConfigRepositoryStructureFiles(repositoryTeams []setupRepositoryTeamInput, repositories []string) map[string]string {
	files := map[string]string{}
	repositoryTeams = normalizeSetupRepositoryTeams(repositoryTeams, repositories)
	for _, team := range repositoryTeams {
		files[configRepositoryTeamStructurePathForScope(team.Name)] = setupConfigRepositoryTeamStructureYAML(team)
	}
	return files
}

func setupConfigRepositoryTeamStructureYAML(team setupRepositoryTeamInput) string {
	var builder strings.Builder
	builder.WriteString("description: Repository team\n")
	if len(team.Repositories) == 0 {
		builder.WriteString("apps: []\n")
		return builder.String()
	}
	builder.WriteString("apps:\n")
	for _, repo := range team.Repositories {
		fmt.Fprintf(&builder, "  - name: %s\n", configsync.RepositoryDisplayNameFromFullName(repo))
		fmt.Fprintf(&builder, "    repo_url: %s\n", configsync.CanonicalRepositoryURL(repo))
	}
	return builder.String()
}

func randomSecret(size int) (string, error) {
	if size <= 0 {
		size = 32
	}
	buf := make([]byte, size)
	if _, err := crand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
