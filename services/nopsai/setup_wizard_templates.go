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
		{ID: setupProfileDev, Label: "Dev", Description: "Local evaluation with starter folders, a smoke pipeline, and generated local values."},
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
	for _, raw := range values["repository"] {
		repositories = append(repositories, raw)
	}
	for _, raw := range values["repositories"] {
		for _, part := range strings.Split(raw, ",") {
			repositories = append(repositories, part)
		}
	}
	return normalizeSetupRepositories(repositories)
}

func setupTemplateOptionsFromQuery(values url.Values) setupTemplateOptions {
	includeLLM := queryBoolDefault(values, "include_llm", true)
	includeMCP := queryBoolDefault(values, "mcp_examples", true)
	return setupTemplateOptions{
		RepositoryGroups: setupRepositoryGroupsFromQuery(values),
		Users:            setupUsersFromQuery(values),
		IncludeLLM:       includeLLM,
		IncludeMCP:       includeMCP,
		LLMProfile: setupLLMProfileInput{
			Name:         "standard",
			Provider:     values.Get("llm_provider"),
			Model:        values.Get("llm_model"),
			BaseURL:      values.Get("llm_base_url"),
			APIKeySecret: values.Get("llm_api_key_secret"),
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

func setupRepositoryGroupsFromQuery(values url.Values) []setupRepositoryGroupInput {
	var groups []setupRepositoryGroupInput
	for _, raw := range values["repository_group"] {
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
		var repositories []string
		for _, repo := range strings.Split(reposText, ",") {
			repositories = append(repositories, repo)
		}
		groups = append(groups, setupRepositoryGroupInput{
			Name:         name,
			Repositories: repositories,
		})
	}
	return normalizeSetupRepositoryGroups(groups, nil)
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
		user.Sub = strings.TrimSpace(user.Sub)
		user.Email = strings.TrimSpace(user.Email)
		user.Role = strings.TrimSpace(user.Role)
		user.Group = normalizeSetupRepositoryGroupName(user.Group)
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

func normalizeSetupRepositoryGroups(raw []setupRepositoryGroupInput, legacyRepositories []string) []setupRepositoryGroupInput {
	if len(raw) == 0 && len(legacyRepositories) > 0 {
		raw = []setupRepositoryGroupInput{{
			Name:         "applications",
			Repositories: legacyRepositories,
		}}
	}

	groupsByName := map[string][]string{}
	var groupOrder []string
	for _, group := range raw {
		name := normalizeSetupRepositoryGroupName(group.Name)
		if name == "" {
			continue
		}
		if _, exists := groupsByName[name]; !exists {
			groupOrder = append(groupOrder, name)
		}
		groupsByName[name] = append(groupsByName[name], normalizeSetupRepositories(group.Repositories)...)
	}
	if len(groupsByName) == 0 {
		return nil
	}

	result := make([]setupRepositoryGroupInput, 0, len(groupOrder))
	seenRepositories := map[string]bool{}
	for _, name := range groupOrder {
		repositories := normalizeSetupRepositories(groupsByName[name])
		filtered := make([]string, 0, len(repositories))
		for _, repo := range repositories {
			key := strings.ToLower(repo)
			if seenRepositories[key] {
				continue
			}
			seenRepositories[key] = true
			filtered = append(filtered, repo)
		}
		result = append(result, setupRepositoryGroupInput{Name: name, Repositories: filtered})
	}
	return result
}

func normalizeSetupRepositoryGroupName(raw string) string {
	name := strings.Trim(strings.TrimSpace(raw), "/")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.Join(strings.Fields(name), "-")
	if name == "" || strings.Contains(name, "..") {
		return ""
	}
	return name
}

func setupRepositoriesFromGroups(groups []setupRepositoryGroupInput) []string {
	var repositories []string
	for _, group := range groups {
		repositories = append(repositories, group.Repositories...)
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
	repositoryGroups := normalizeSetupRepositoryGroups(options.RepositoryGroups, repositories)
	if len(repositoryGroups) > 0 {
		repositories = setupRepositoriesFromGroups(repositoryGroups)
	} else {
		repositories = normalizeSetupRepositories(repositories)
		repositoryGroups = normalizeSetupRepositoryGroups(nil, repositories)
	}
	files := map[string]string{
		"README.md":                                 setupReadme(profile),
		"pipelines/setup/first-run.yaml":            setupFirstRunPipelineYAML(profile),
		"steps/setup/announce.yaml":                 setupReusableStepYAML(),
		"scopes/dev/scope.yaml":                     setupScopeYAML(profile, "dev"),
		"knowledge/guideline/platform/setup-run.md": setupKnowledgeMarkdown(profile),
		"access/bootstrap.yaml":                     setupAccessYAML(profile, repositoryGroups, options.Users),
	}
	for path, content := range setupConfigRepositoryStructureFiles(repositoryGroups, repositories) {
		files[path] = content
	}
	if options.IncludeLLM {
		files["setting/system/llm_profile.yaml"] = setupLLMProfileYAML(options.LLMProfile)
	}
	if options.IncludeMCP {
		files["setting/system/mcp.yaml"] = setupMCPYAML()
	}
	if profile == setupProfileTeam || profile == setupProfileProduction {
		files["scopes/prod/scope.yaml"] = setupScopeYAML(profile, "prod")
	}
	for _, repo := range repositories {
		files["triggers/"+repo+".yaml"] = setupTriggerYAML(profile)
	}
	return files
}

func setupPipelineRunStructure(profile string, repositoryGroups []setupRepositoryGroupInput, repositories []string) map[string]*configsync.PipelineRunStructureNode {
	root := setupAccessFolder(profile)
	node := &configsync.PipelineRunStructureNode{
		Description: "Starter workspace",
		Children:    map[string]*configsync.PipelineRunStructureNode{},
	}
	repositoryGroups = normalizeSetupRepositoryGroups(repositoryGroups, repositories)
	for _, group := range repositoryGroups {
		child := &configsync.PipelineRunStructureNode{Description: "Repository group " + group.Name, Children: map[string]*configsync.PipelineRunStructureNode{}}
		for _, repo := range group.Repositories {
			parts := strings.Split(repo, "/")
			if len(parts) != 2 {
				continue
			}
			child.Apps = append(child.Apps, configsync.PipelineRunStructureApp{
				Name:               configsync.RepositoryDisplayNameFromFullName(repo),
				RepoURL:            configsync.CanonicalRepositoryURL(repo),
				RepositoryFullName: repo,
			})
			child.Repos = append(child.Repos, repo)
		}
		node.Children[group.Name] = child
	}
	return map[string]*configsync.PipelineRunStructureNode{root: node}
}

func setupAccessFolder(profile string) string {
	switch normalizeSetupProfile(profile) {
	case setupProfileTeam:
		return "workspace"
	case setupProfileProduction:
		return "production"
	default:
		return "sandbox"
	}
}

func setupScopeVariables(profile string) map[string]map[string]string {
	workspace := setupAccessFolder(profile)
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
	group       string
	name        string
	description string
	content     string
}

func setupKnowledgeContexts(profile string) []setupKnowledgeContextSeed {
	return []setupKnowledgeContextSeed{{
		kind:        "guideline",
		group:       "platform",
		name:        "setup-run",
		description: "Starter pipeline run expectations",
		content:     fmt.Sprintf("Use the starter pipeline in %s to verify runner connectivity, log streaming, and optional LLM execution before attaching production repositories.", setupAccessFolder(profile)),
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

func setupFirstRunPipelineYAML(profile string) string {
	scope := "dev"
	if normalizeSetupProfile(profile) == setupProfileProduction {
		scope = "prod"
	}
	return strings.TrimSpace(fmt.Sprintf(`
name: first-run
version: "1.0.0"
description: Verifies that NopsAI can run a starter job, stream logs, and optionally call the configured LLM profile.
container_image: alpine:3.20
working_directory: /workspace
timeout: 10m
llm_profile: standard
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
      - announce

  - name: ai-smoke
    goal: Return one short sentence confirming that the NopsAI setup smoke test reached the agent.
    ignore_failure: true
    depends_on:
      - runner-smoke
`, scope, scope)) + "\n"
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

func setupScopeYAML(profile, scope string) string {
	return strings.TrimSpace(fmt.Sprintf(`
variables:
  NOPSAI_SETUP_WORKSPACE: %s
  NOPSAI_SETUP_SCOPE: %s
secrets:
  GEMINI_API_KEY:
`, setupAccessFolder(profile), scope)) + "\n"
}

func setupReadme(profile string) string {
	return fmt.Sprintf("# NopsAI starter config\n\nWorkspace: `%s`\n\nThis repository contains starter resources for the first NopsAI workspace bootstrap. Keep plaintext secrets outside this repository. Scope files define plaintext scoped values under `variables:` and may define secret keys under `secrets:` with `null` placeholders or encrypted values generated by this NopsAI instance.\n", setupAccessFolder(profile))
}

func setupKnowledgeMarkdown(profile string) string {
	return fmt.Sprintf("---\ndescription: Starter setup run expectations\n---\n\nUse the %s starter workspace to prove runner connectivity, logs, repository triggers, and optional LLM execution before onboarding production automation.\n", setupAccessFolder(profile))
}

func setupAccessYAML(profile string, repositoryGroups []setupRepositoryGroupInput, users []setupUserInput) string {
	folders := setupAccessGrantFolders(profile, repositoryGroups)
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
			builder.WriteString(fmt.Sprintf("  - sub: %q\n", sub))
			builder.WriteString(fmt.Sprintf("    email: %q\n", email))
			builder.WriteString("    provider: local\n")
			builder.WriteString("    status: active\n")
			builder.WriteString("    # password is intentionally not generated into GitOps; set it out of band if GitOps creates the account.\n")
		}
	}
	builder.WriteString("\nbasic_roles:\n")
	for _, folder := range folders {
		builder.WriteString("  - user: admin\n")
		builder.WriteString("    role: owner\n")
		builder.WriteString(fmt.Sprintf("    resource: folder:%s\n", folder))
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
		folder := normalizeSetupRepositoryGroupName(user.Group)
		if folder == "" {
			folder = folders[0]
		}
		builder.WriteString(fmt.Sprintf("  - user: %q\n", sub))
		builder.WriteString(fmt.Sprintf("    role: %s\n", role))
		builder.WriteString(fmt.Sprintf("    resource: folder:%s\n", folder))
	}
	return builder.String()
}

func setupAccessGrantFolders(profile string, repositoryGroups []setupRepositoryGroupInput) []string {
	seen := map[string]bool{}
	var folders []string
	for _, group := range repositoryGroups {
		name := normalizeSetupRepositoryGroupName(group.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		folders = append(folders, name)
	}
	if len(folders) == 0 {
		folders = append(folders, setupAccessFolder(profile))
	}
	return folders
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
	apiKeySecret := strings.TrimSpace(input.APIKeySecret)
	if apiKeySecret == "" {
		apiKeySecret = config.DefaultLLMProviderAPIKeySecret(provider)
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
			APIKeySecret:   apiKeySecret,
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
    auth_secret: GITHUB_MCP_TOKEN
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

func setupConfigRepositoryStructureFiles(repositoryGroups []setupRepositoryGroupInput, repositories []string) map[string]string {
	files := map[string]string{}
	repositoryGroups = normalizeSetupRepositoryGroups(repositoryGroups, repositories)
	for _, group := range repositoryGroups {
		files[configRepositoryGroupStructurePathForScope(group.Name)] = setupConfigRepositoryGroupStructureYAML(group)
	}
	return files
}

func setupConfigRepositoryGroupStructureYAML(group setupRepositoryGroupInput) string {
	var builder strings.Builder
	builder.WriteString("description: Repository group\n")
	if len(group.Repositories) == 0 {
		builder.WriteString("apps: []\n")
		return builder.String()
	}
	builder.WriteString("apps:\n")
	for _, repo := range group.Repositories {
		builder.WriteString(fmt.Sprintf("  - name: %s\n", configsync.RepositoryDisplayNameFromFullName(repo)))
		builder.WriteString(fmt.Sprintf("    repo_url: %s\n", configsync.CanonicalRepositoryURL(repo)))
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
