package nopsai

import (
	"fmt"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

type generalScopeVarKey struct {
	scopePath string
	name      string
}

type repoScopeVarKey struct {
	repo      string
	scopePath string
	name      string
}

type generalScopeSecretKey struct {
	scopePath string
	name      string
}

type repoScopeSecretKey struct {
	repo      string
	scopePath string
	name      string
}

type storedPipeline struct {
	definition string
	version    string
	path       string
	name       string
	sourcePath string
}

type storedStep struct {
	definition string
	path       string
	name       string
	sourcePath string
}

type storedConfigRepository struct {
	scopeType     string
	scopeID       string
	provider      string
	repoURL       string
	branch        string
	basePath      string
	credentialRef string
	enabled       bool
	writeEnabled  bool
	writeBranch   string
	sourcePath    string
}

type storedScopeVar struct {
	value      string
	sourcePath string
}

type storedScopeSecret struct {
	encryptedValue *string
	sourcePath     string
}

func (a *App) addScopeConfigEntries(
	raw map[string]interface{},
	generalScopeVars map[generalScopeVarKey]storedScopeVar,
	repoScopeVars map[repoScopeVarKey]storedScopeVar,
	generalScopeSecrets map[generalScopeSecretKey]storedScopeSecret,
	repoScopeSecrets map[repoScopeSecretKey]storedScopeSecret,
	scopePath string,
	sourcePath string,
	binding models.ConfigRepository,
	boundTeam string,
) (bool, error) {
	hasEmbeddedScopeAccess := false
	for key, value := range raw {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			return false, fmt.Errorf("scope file '%s' contains an empty key", sourcePath)
		}

		switch trimmedKey {
		case "access":
			hasEmbeddedScopeAccess = true
			continue
		case "variables":
			variables, ok := scopeVariablesSection(value)
			if !ok {
				return false, fmt.Errorf("scope variables section in '%s' must be a map of variable names to string values", sourcePath)
			}
			for variableKey, variableValue := range variables {
				if err := addScopeVariableConfigEntry(generalScopeVars, repoScopeVars, scopePath, variableKey, variableValue, sourcePath, binding, boundTeam); err != nil {
					return false, err
				}
			}
		case "secrets":
			secrets, ok := scopeVariablesSection(value)
			if !ok {
				return false, fmt.Errorf("scope secrets section in '%s' must be a map of secret names to encrypted string values or null placeholders", sourcePath)
			}
			for secretKey, secretValue := range secrets {
				if err := a.addScopeSecretConfigEntry(generalScopeSecrets, repoScopeSecrets, scopePath, secretKey, secretValue, sourcePath, binding, boundTeam); err != nil {
					return false, err
				}
			}
		default:
			return false, fmt.Errorf("scope file '%s' contains unsupported top-level key '%s'; define variables under the variables section", sourcePath, trimmedKey)
		}
	}
	return hasEmbeddedScopeAccess, nil
}

func scopeVariablesSection(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[any]any:
		normalized := make(map[string]any, len(typed))
		for key, value := range typed {
			keyString, ok := key.(string)
			if !ok {
				return nil, false
			}
			normalized[keyString] = value
		}
		return normalized, true
	default:
		return nil, false
	}
}

func addScopeVariableConfigEntry(
	generalScopeVars map[generalScopeVarKey]storedScopeVar,
	repoScopeVars map[repoScopeVarKey]storedScopeVar,
	scopePath string,
	rawKey string,
	value any,
	sourcePath string,
	binding models.ConfigRepository,
	boundTeam string,
) error {
	trimmedKey := strings.TrimSpace(rawKey)
	if trimmedKey == "" {
		return fmt.Errorf("scope file '%s' contains an empty variable key", sourcePath)
	}
	strValue, ok := value.(string)
	if !ok {
		return fmt.Errorf("scope entry '%s' in '%s' must be a string", trimmedKey, sourcePath)
	}

	parts := strings.Split(trimmedKey, "/")
	switch {
	case len(parts) == 1:
		if err := validateScopeRuntimeName("variable", trimmedKey, trimmedKey, sourcePath); err != nil {
			return err
		}
		gKey := generalScopeVarKey{scopePath: scopePath, name: trimmedKey}
		if _, exists := generalScopeVars[gKey]; exists {
			return fmt.Errorf("duplicate scope variable '%s' for '%s' detected", trimmedKey, scopePath)
		}
		generalScopeVars[gKey] = storedScopeVar{value: strValue, sourcePath: sourcePath}
	case len(parts) >= 3:
		repoName, varName, err := parseRepositoryScopeRuntimeKey("variable", trimmedKey, sourcePath, binding, boundTeam)
		if err != nil {
			return err
		}
		rKey := repoScopeVarKey{repo: repoName, scopePath: scopePath, name: varName}
		if _, exists := repoScopeVars[rKey]; exists {
			return fmt.Errorf("duplicate repository scope variable '%s' for '%s' detected", trimmedKey, scopePath)
		}
		repoScopeVars[rKey] = storedScopeVar{value: strValue, sourcePath: sourcePath}
	default:
		return fmt.Errorf("scope key '%s' in '%s' has an unsupported format", trimmedKey, sourcePath)
	}
	return nil
}

func (a *App) addScopeSecretConfigEntry(
	generalScopeSecrets map[generalScopeSecretKey]storedScopeSecret,
	repoScopeSecrets map[repoScopeSecretKey]storedScopeSecret,
	scopePath string,
	rawKey string,
	value any,
	sourcePath string,
	binding models.ConfigRepository,
	boundTeam string,
) error {
	trimmedKey := strings.TrimSpace(rawKey)
	if trimmedKey == "" {
		return fmt.Errorf("scope file '%s' contains an empty secret key", sourcePath)
	}
	encryptedValue, err := a.gitOpsSecretEncryptedValue(value, trimmedKey, sourcePath)
	if err != nil {
		return err
	}

	parts := strings.Split(trimmedKey, "/")
	switch {
	case len(parts) == 1:
		if err := validateScopeRuntimeName("secret", trimmedKey, trimmedKey, sourcePath); err != nil {
			return err
		}
		gKey := generalScopeSecretKey{scopePath: scopePath, name: trimmedKey}
		if _, exists := generalScopeSecrets[gKey]; exists {
			return fmt.Errorf("duplicate scope secret '%s' for '%s' detected", trimmedKey, scopePath)
		}
		generalScopeSecrets[gKey] = storedScopeSecret{encryptedValue: encryptedValue, sourcePath: sourcePath}
	case len(parts) >= 3:
		repoName, secretName, err := parseRepositoryScopeRuntimeKey("secret", trimmedKey, sourcePath, binding, boundTeam)
		if err != nil {
			return err
		}
		rKey := repoScopeSecretKey{repo: repoName, scopePath: scopePath, name: secretName}
		if _, exists := repoScopeSecrets[rKey]; exists {
			return fmt.Errorf("duplicate repository scope secret '%s' for '%s' detected", trimmedKey, scopePath)
		}
		repoScopeSecrets[rKey] = storedScopeSecret{encryptedValue: encryptedValue, sourcePath: sourcePath}
	default:
		return fmt.Errorf("scope secret key '%s' in '%s' has an unsupported format", trimmedKey, sourcePath)
	}
	return nil
}

func parseRepositoryScopeRuntimeKey(kind, rawKey, sourcePath string, binding models.ConfigRepository, boundTeam string) (string, string, error) {
	parts := strings.Split(rawKey, "/")
	runtimeName := strings.TrimSpace(parts[len(parts)-1])
	if runtimeName == "" {
		return "", "", fmt.Errorf("invalid repository-scoped %s key '%s' in '%s'", kind, rawKey, sourcePath)
	}
	if err := validateScopeRuntimeName(kind, runtimeName, rawKey, sourcePath); err != nil {
		return "", "", err
	}

	repoSegments, err := configsync.CleanPathSegments(strings.Join(parts[:len(parts)-1], "/"), false)
	if err != nil {
		return "", "", fmt.Errorf("invalid repository-scoped %s key '%s' in '%s': %w", kind, rawKey, sourcePath, err)
	}
	repoName := strings.Join(repoSegments, "/")
	if binding.ScopeType == models.ConfigRepositoryScopeTeam {
		normalizedRepoName, err := configsync.NormalizePathForTeam(boundTeam, repoName)
		if err != nil {
			return "", "", fmt.Errorf("invalid team-scoped repository %s key '%s' in '%s': %w", kind, rawKey, sourcePath, err)
		}
		repoName = normalizedRepoName
	}
	return repoName, runtimeName, nil
}

func validateScopeRuntimeName(kind, name, rawKey, sourcePath string) error {
	if !models.IsValidRuntimeReferenceName(name) {
		return fmt.Errorf("scope %s key '%s' in '%s' has invalid runtime name '%s'; names must match ^[A-Za-z0-9_.-]+$", kind, rawKey, sourcePath, name)
	}
	return nil
}

func (a *App) gitOpsSecretEncryptedValue(value any, secretKey, sourcePath string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	strValue, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("scope secret entry '%s' in '%s' must be an encrypted string or null", secretKey, sourcePath)
	}
	strValue = strings.TrimSpace(strValue)
	if strValue == "" {
		return nil, nil
	}
	if _, err := a.decrypt(strValue); err != nil {
		return nil, nil
	}
	return &strValue, nil
}

type storedTrigger struct {
	definition string
	sourcePath string
	record     repositoryTriggerRecord
}

func configRepositoryBindingsFromPipelineRunStructure(structure map[string]*configsync.PipelineRunStructureNode, sourcePath string) (map[string]storedConfigRepository, error) {
	result := map[string]storedConfigRepository{}

	var walk func(path []string, node *configsync.PipelineRunStructureNode) error
	walk = func(path []string, node *configsync.PipelineRunStructureNode) error {
		if node == nil {
			return nil
		}
		scopeID := strings.Trim(strings.Join(path, "/"), "/")
		if node.Config != nil {
			if scopeID == "" {
				return fmt.Errorf("config repository binding '%s' is missing a team path", sourcePath)
			}
			if _, err := configsync.CleanPathSegments(scopeID, false); err != nil {
				return fmt.Errorf("invalid config repository binding '%s': %w", sourcePath, err)
			}
			file := *node.Config
			if err := configsync.ValidateBindingFile(file, models.ConfigRepositoryScopeTeam, scopeID, sourcePath); err != nil {
				return err
			}
			basePath, err := configsync.NormalizeRepositoryBasePathForRequest(file.BasePath)
			if err != nil {
				return fmt.Errorf("invalid base_path in config repository binding '%s': %w", sourcePath, err)
			}
			enabled := true
			if file.Enabled != nil {
				enabled = *file.Enabled
			}
			writeEnabled, writeBranch := configsync.BindingWriteSettings(file)
			branch := strings.TrimSpace(file.Branch)
			if branch == "" {
				branch = "main"
			}
			provider, err := configsync.NormalizeRepositoryProvider(file.Provider, file.RepoURL)
			if err != nil {
				return err
			}

			key := models.ConfigRepositoryScopeTeam + "/" + scopeID
			if _, exists := result[key]; exists {
				return fmt.Errorf("duplicate config repository binding for '%s' detected", key)
			}
			result[key] = storedConfigRepository{
				scopeType:     models.ConfigRepositoryScopeTeam,
				scopeID:       scopeID,
				provider:      provider,
				repoURL:       strings.TrimSpace(file.RepoURL),
				branch:        branch,
				basePath:      basePath,
				credentialRef: strings.TrimSpace(file.CredentialRef),
				enabled:       enabled,
				writeEnabled:  writeEnabled,
				writeBranch:   writeBranch,
				sourcePath:    sourcePath,
			}
		}

		for childName, childNode := range node.Children {
			childSegments, err := configsync.CleanPathSegments(childName, false)
			if err != nil {
				return err
			}
			if err := walk(append(append([]string{}, path...), childSegments...), childNode); err != nil {
				return err
			}
		}
		return nil
	}

	for name, node := range structure {
		segments, err := configsync.CleanPathSegments(name, false)
		if err != nil {
			return nil, err
		}
		if err := walk(segments, node); err != nil {
			return nil, err
		}
	}
	return result, nil
}
