package nopsai

import (
	"context"
	"fmt"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/pkg/validation"

	"github.com/rs/zerolog/log"
)

func validatePipeline(pipeline *models.Pipeline) error {
	return validation.ValidatePipeline(pipeline)
}

func (a *App) prepareSecretsForPipeline(runID string, pipeline models.Pipeline, gitContext map[string]string, scope string) (map[string]string, error) {
	requiredSecrets := make(map[string]models.ScopedRuntimeRef)
	for _, step := range pipeline.Steps {
		stepSecretNames := make(map[string]string)
		for _, rawSecretName := range step.GetSecrets() {
			if strings.TrimSpace(rawSecretName) == "" {
				continue
			}
			secretRef, err := models.ParseScopedRuntimeRef(rawSecretName, scope)
			if err != nil {
				return nil, fmt.Errorf("pipeline aborted: invalid secret reference '%s': %w", rawSecretName, err)
			}
			if previousLookup, ok := stepSecretNames[secretRef.Name]; ok && previousLookup != secretRef.LookupKey() {
				stepName := strings.TrimSpace(step.GetName())
				if stepName == "" {
					stepName = "unknown"
				}
				return nil, fmt.Errorf("pipeline aborted: secret references in step '%s' resolve to multiple values for runtime name '%s'", stepName, secretRef.Name)
			}
			stepSecretNames[secretRef.Name] = secretRef.LookupKey()
			requiredSecrets[secretRef.Key()] = secretRef
		}
	}

	if len(requiredSecrets) == 0 {
		return nil, nil
	}

	finalSecrets := make(map[string]string)
	repoFullName := fmt.Sprintf("%s/%s", gitContext["repo_owner"], gitContext["repo_name"])

	for secretKey, secretRef := range requiredSecrets {
		encryptedValue, resourceID, found, err := a.findEncryptedSecret(secretRef.Name, repoFullName, secretRef.Scope)
		if err != nil {
			return nil, fmt.Errorf("pipeline aborted: failed to resolve secret '%s': %w", secretKey, err)
		}
		if !found {
			if secretRef.Scope != "" {
				return nil, fmt.Errorf("pipeline aborted: required secret '%s' not found for scope '%s'", secretKey, secretRef.DisplayScope())
			}
			return nil, fmt.Errorf("pipeline aborted: required secret '%s' not found in the default scope", secretKey)
		}
		if strings.TrimSpace(runID) != "" {
			if err := a.authorizeRunRuntimeResourceUse(context.Background(), runID, gitContext, "secret.use", grantResourceSecret, resourceID); err != nil {
				return nil, fmt.Errorf("pipeline aborted: %w", err)
			}
		}
		if strings.TrimSpace(encryptedValue) == "" {
			return nil, fmt.Errorf("pipeline aborted: required secret '%s' has no value", secretKey)
		}

		decryptedValue, decryptErr := a.decrypt(encryptedValue)
		if decryptErr != nil {
			log.Error().Err(decryptErr).Str("secret_name", secretKey).Msg("Failed to decrypt secret; this will cause a failure.")
			return nil, fmt.Errorf("pipeline aborted: failed to decrypt secret '%s'", secretKey)
		}
		finalSecrets[secretKey] = decryptedValue
	}

	return finalSecrets, nil
}
