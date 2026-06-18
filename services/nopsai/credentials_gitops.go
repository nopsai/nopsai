package nopsai

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/credentials"
)

type gitOpsCredentialDirectory struct {
	root  string
	files map[string]string
}

type gitOpsCredentialPlan struct {
	credentials []gitOpsCredential
	sourcePath  string
}

type gitOpsCredentialFile struct {
	Credentials []gitOpsCredential `json:"credentials" yaml:"credentials"`
}

type gitOpsCredential struct {
	Reference     string                  `json:"reference" yaml:"reference"`
	Kind          string                  `json:"kind" yaml:"kind"`
	Description   string                  `json:"description,omitempty" yaml:"description,omitempty"`
	Status        string                  `json:"status,omitempty" yaml:"status,omitempty"`
	ActiveVersion int                     `json:"active_version,omitempty" yaml:"active_version,omitempty"`
	ExpiresAt     *time.Time              `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	Versions      []gitOpsCredentialValue `json:"versions,omitempty" yaml:"versions,omitempty"`
}

type gitOpsCredentialValue struct {
	Version                 int    `json:"version" yaml:"version"`
	EncryptionFormatVersion int    `json:"encryption_format_version" yaml:"encryption_format_version"`
	EncryptionKeyID         string `json:"encryption_key_id" yaml:"encryption_key_id"`
	Ciphertext              string `json:"ciphertext" yaml:"ciphertext"`
	WrappedDataKey          string `json:"wrapped_data_key" yaml:"wrapped_data_key"`
}

type credentialGitOpsQuerier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func parseGitOpsCredentialPlan(binding models.ConfigRepository, directories ...gitOpsCredentialDirectory) (*gitOpsCredentialPlan, error) {
	var candidates []gitOpsRuntimeSettingsFileCandidate
	for _, directory := range directories {
		root := filepath.ToSlash(strings.Trim(directory.root, "/"))
		for path, content := range directory.files {
			normalized := filepath.ToSlash(path)
			rel, ok := configsync.RelativePath(normalized, root)
			if !ok || !isGitOpsCredentialsRelativePath(rel) {
				continue
			}
			candidates = append(candidates, gitOpsRuntimeSettingsFileCandidate{
				sourcePath: normalized,
				content:    content,
			})
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if binding.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil, fmt.Errorf("credentials can only be configured from a system config repository")
	}
	if len(candidates) > 1 {
		paths := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			paths = append(paths, candidate.sourcePath)
		}
		sort.Strings(paths)
		return nil, fmt.Errorf("multiple credential GitOps files found: %s", strings.Join(paths, ", "))
	}
	return parseGitOpsCredentialFile(candidates[0].content, candidates[0].sourcePath)
}

func isGitOpsCredentialsRelativePath(rel string) bool {
	return strings.Trim(filepath.ToSlash(rel), "/") == "system/credentials.yaml"
}

func parseGitOpsCredentialFile(content, sourcePath string) (*gitOpsCredentialPlan, error) {
	var file gitOpsCredentialFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return nil, fmt.Errorf("failed to parse credential GitOps file '%s': %w", sourcePath, err)
	}
	seen := map[string]struct{}{}
	for idx := range file.Credentials {
		credential := &file.Credentials[idx]
		ref, err := credentials.ParseReference(credential.Reference)
		if err != nil {
			return nil, fmt.Errorf("credential GitOps file '%s' has invalid reference %q: %w", sourcePath, credential.Reference, err)
		}
		credential.Reference = ref.String()
		if _, exists := seen[credential.Reference]; exists {
			return nil, fmt.Errorf("credential GitOps file '%s' contains duplicate credential %q", sourcePath, credential.Reference)
		}
		seen[credential.Reference] = struct{}{}
		credentialKind, err := normalizeCredentialKind(credential.Kind)
		if err != nil {
			return nil, fmt.Errorf("credential GitOps file '%s' credential %q has invalid kind: %w", sourcePath, credential.Reference, err)
		}
		credential.Kind = credentialKind
		credential.Description = strings.TrimSpace(credential.Description)
		credential.Status = normalizeCredentialGitOpsStatus(credential.Status, credential.ActiveVersion)
		if credential.ActiveVersion < 0 {
			return nil, fmt.Errorf("credential GitOps file '%s' credential %q has invalid active_version", sourcePath, credential.Reference)
		}
		if credential.Status == credentials.StatusActive && credential.ActiveVersion <= 0 {
			return nil, fmt.Errorf("credential GitOps file '%s' credential %q is active without active_version", sourcePath, credential.Reference)
		}
		versions := map[int]struct{}{}
		for versionIdx := range credential.Versions {
			version := &credential.Versions[versionIdx]
			if version.Version <= 0 {
				return nil, fmt.Errorf("credential GitOps file '%s' credential %q has invalid version", sourcePath, credential.Reference)
			}
			if _, exists := versions[version.Version]; exists {
				return nil, fmt.Errorf("credential GitOps file '%s' credential %q contains duplicate version %d", sourcePath, credential.Reference, version.Version)
			}
			versions[version.Version] = struct{}{}
			if version.EncryptionFormatVersion <= 0 {
				return nil, fmt.Errorf("credential GitOps file '%s' credential %q version %d has invalid encryption_format_version", sourcePath, credential.Reference, version.Version)
			}
			version.EncryptionKeyID = strings.TrimSpace(version.EncryptionKeyID)
			if version.EncryptionKeyID == "" {
				return nil, fmt.Errorf("credential GitOps file '%s' credential %q version %d is missing encryption_key_id", sourcePath, credential.Reference, version.Version)
			}
			if _, err := decodeGitOpsCredentialBytes(version.Ciphertext); err != nil {
				return nil, fmt.Errorf("credential GitOps file '%s' credential %q version %d has invalid ciphertext: %w", sourcePath, credential.Reference, version.Version, err)
			}
			if _, err := decodeGitOpsCredentialBytes(version.WrappedDataKey); err != nil {
				return nil, fmt.Errorf("credential GitOps file '%s' credential %q version %d has invalid wrapped_data_key: %w", sourcePath, credential.Reference, version.Version, err)
			}
		}
		if credential.ActiveVersion > 0 {
			if _, exists := versions[credential.ActiveVersion]; !exists {
				return nil, fmt.Errorf("credential GitOps file '%s' credential %q active_version %d is not defined", sourcePath, credential.Reference, credential.ActiveVersion)
			}
		}
		sort.Slice(credential.Versions, func(i, j int) bool {
			return credential.Versions[i].Version < credential.Versions[j].Version
		})
	}
	sort.Slice(file.Credentials, func(i, j int) bool {
		return file.Credentials[i].Reference < file.Credentials[j].Reference
	})
	return &gitOpsCredentialPlan{credentials: file.Credentials, sourcePath: sourcePath}, nil
}

func normalizeCredentialGitOpsStatus(raw string, activeVersion int) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case credentials.StatusActive:
		return credentials.StatusActive
	case credentials.StatusDisabled:
		return credentials.StatusDisabled
	case credentials.StatusPending:
		return credentials.StatusPending
	default:
		if activeVersion > 0 {
			return credentials.StatusActive
		}
		return credentials.StatusPending
	}
}

func decodeGitOpsCredentialBytes(value string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("decoded value is empty")
	}
	return decoded, nil
}

func encodeGitOpsCredentialBytes(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(value)
}

func buildGitOpsCredentialDocument(records []credentials.Credential, versionsByCredential map[uuid.UUID][]credentials.Version) gitOpsCredentialFile {
	doc := gitOpsCredentialFile{}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Reference.String() < records[j].Reference.String()
	})
	for _, record := range records {
		kind, err := normalizeCredentialKind(record.Kind)
		if err != nil {
			continue
		}
		item := gitOpsCredential{
			Reference:     record.Reference.String(),
			Kind:          kind,
			Description:   strings.TrimSpace(record.Description),
			Status:        normalizeCredentialGitOpsStatus(record.Status, record.ActiveVersion),
			ActiveVersion: record.ActiveVersion,
			ExpiresAt:     record.ExpiresAt,
		}
		versions := append([]credentials.Version(nil), versionsByCredential[record.ID]...)
		sort.Slice(versions, func(i, j int) bool {
			return versions[i].Version < versions[j].Version
		})
		for _, version := range versions {
			if version.RevokedAt != nil {
				continue
			}
			item.Versions = append(item.Versions, gitOpsCredentialValue{
				Version:                 version.Version,
				EncryptionFormatVersion: version.Envelope.EncryptionFormatVersion,
				EncryptionKeyID:         strings.TrimSpace(version.Envelope.EncryptionKeyID),
				Ciphertext:              encodeGitOpsCredentialBytes(version.Envelope.Ciphertext),
				WrappedDataKey:          encodeGitOpsCredentialBytes(version.Envelope.WrappedDataKey),
			})
		}
		doc.Credentials = append(doc.Credentials, item)
	}
	return doc
}

func (a *App) exportConfigRepositoryCredentials(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
	if repo.ScopeType != models.ConfigRepositoryScopeSystem || a == nil || a.credentialStore == nil {
		return nil
	}
	records, err := a.credentialStore.ListCredentials(ctx)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	versionsByCredential := map[uuid.UUID][]credentials.Version{}
	for _, record := range records {
		versions, err := a.credentialStore.ListCredentialVersions(ctx, record.ID)
		if err != nil {
			return err
		}
		versionsByCredential[record.ID] = versions
	}
	content, err := marshalConfigRepositoryYAML(buildGitOpsCredentialDocument(records, versionsByCredential))
	if err != nil {
		return err
	}
	files[configRepositoryCredentialsPath] = string(content)
	return nil
}

func syncCredentialsFromGitOps(ctx context.Context, db credentialGitOpsQuerier, binding models.ConfigRepository, plan *gitOpsCredentialPlan, commitSHA string) error {
	if db == nil || plan == nil {
		return nil
	}
	keepRefs := make([]string, 0, len(plan.credentials))
	for _, item := range plan.credentials {
		ref, err := credentials.ParseReference(item.Reference)
		if err != nil {
			return err
		}
		credentialID := uuid.New()
		maxVersion := item.ActiveVersion
		for _, version := range item.Versions {
			if version.Version > maxVersion {
				maxVersion = version.Version
			}
		}
		nextVersion := maxVersion + 1
		if nextVersion <= 1 {
			nextVersion = 1
		}
		var id uuid.UUID
		if err := db.QueryRow(ctx, `
			INSERT INTO credentials (
				id, namespace, name, kind, description, status, active_version, next_version,
				expires_at, managed_by_config_repo, config_repo_id, config_source_path,
				config_source_commit_sha, created_by, updated_by
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, TRUE, $10, $11, $12, $13, $13)
			ON CONFLICT (namespace, name) DO UPDATE SET
				kind = EXCLUDED.kind,
				description = EXCLUDED.description,
				status = EXCLUDED.status,
				active_version = EXCLUDED.active_version,
				next_version = GREATEST(credentials.next_version, EXCLUDED.next_version),
				expires_at = EXCLUDED.expires_at,
				managed_by_config_repo = TRUE,
				config_repo_id = EXCLUDED.config_repo_id,
				config_source_path = EXCLUDED.config_source_path,
				config_source_commit_sha = EXCLUDED.config_source_commit_sha,
				updated_by = EXCLUDED.updated_by,
				updated_at = NOW()
			RETURNING id
		`,
			credentialID,
			ref.Namespace,
			ref.Name,
			item.Kind,
			item.Description,
			item.Status,
			item.ActiveVersion,
			nextVersion,
			item.ExpiresAt,
			binding.ID,
			plan.sourcePath,
			commitSHA,
			fmt.Sprintf("gitops:config-repository:%d", binding.ID),
		).Scan(&id); err != nil {
			return fmt.Errorf("upsert credential %q: %w", item.Reference, err)
		}
		keepRefs = append(keepRefs, ref.String())
		versionNumbers := make([]int, 0, len(item.Versions))
		for _, version := range item.Versions {
			ciphertext, err := decodeGitOpsCredentialBytes(version.Ciphertext)
			if err != nil {
				return fmt.Errorf("decode credential %q version %d ciphertext: %w", item.Reference, version.Version, err)
			}
			wrappedDataKey, err := decodeGitOpsCredentialBytes(version.WrappedDataKey)
			if err != nil {
				return fmt.Errorf("decode credential %q version %d wrapped data key: %w", item.Reference, version.Version, err)
			}
			var activatedAt any
			if version.Version == item.ActiveVersion && item.ActiveVersion > 0 {
				activatedAt = time.Now().UTC()
			}
			if _, err := db.Exec(ctx, `
				INSERT INTO credential_versions (
					credential_id, version, ciphertext, wrapped_data_key, encryption_key_id,
					encryption_format_version, created_by, activated_at, revoked_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL)
				ON CONFLICT (credential_id, version) DO UPDATE SET
					ciphertext = EXCLUDED.ciphertext,
					wrapped_data_key = EXCLUDED.wrapped_data_key,
					encryption_key_id = EXCLUDED.encryption_key_id,
					encryption_format_version = EXCLUDED.encryption_format_version,
					created_by = EXCLUDED.created_by,
					activated_at = COALESCE(credential_versions.activated_at, EXCLUDED.activated_at),
					revoked_at = NULL
			`, id, version.Version, ciphertext, wrappedDataKey, version.EncryptionKeyID, version.EncryptionFormatVersion, fmt.Sprintf("gitops:config-repository:%d", binding.ID), activatedAt); err != nil {
				return fmt.Errorf("upsert credential %q version %d: %w", item.Reference, version.Version, err)
			}
			versionNumbers = append(versionNumbers, version.Version)
		}
		if len(versionNumbers) == 0 {
			if _, err := db.Exec(ctx, `DELETE FROM credential_versions WHERE credential_id = $1`, id); err != nil {
				return fmt.Errorf("prune credential %q versions: %w", item.Reference, err)
			}
		} else if _, err := db.Exec(ctx, `
			DELETE FROM credential_versions
			WHERE credential_id = $1
			  AND version <> ALL($2::int[])
		`, id, versionNumbers); err != nil {
			return fmt.Errorf("prune credential %q versions: %w", item.Reference, err)
		}
	}
	if len(keepRefs) == 0 {
		if _, err := db.Exec(ctx, `
			DELETE FROM credentials
			WHERE managed_by_config_repo = TRUE
			  AND config_repo_id = $1
		`, binding.ID); err != nil {
			return fmt.Errorf("prune credentials: %w", err)
		}
		return nil
	}
	if _, err := db.Exec(ctx, `
		DELETE FROM credentials
		WHERE managed_by_config_repo = TRUE
		  AND config_repo_id = $1
		  AND (namespace || '/' || name) <> ALL($2::text[])
	`, binding.ID, credentialResourceIDs(keepRefs)); err != nil {
		return fmt.Errorf("prune credentials: %w", err)
	}
	return nil
}

func credentialResourceIDs(refs []string) []string {
	result := make([]string, 0, len(refs))
	for _, raw := range refs {
		ref, err := credentials.ParseReference(raw)
		if err == nil {
			result = append(result, ref.ResourceID())
		}
	}
	return result
}
