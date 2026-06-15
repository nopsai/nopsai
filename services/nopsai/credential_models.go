package nopsai

import (
	"time"

	"nopsai/services/nopsai/internal/credentials"
)

type credentialCreateRequest struct {
	Reference   string     `json:"reference"`
	Kind        string     `json:"kind"`
	Description string     `json:"description,omitempty"`
	Value       string     `json:"value,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type credentialValueRequest struct {
	Value string `json:"value"`
}

type credentialResponse struct {
	ID                    string                      `json:"id"`
	Reference             string                      `json:"reference"`
	Kind                  string                      `json:"kind"`
	Description           string                      `json:"description,omitempty"`
	Status                string                      `json:"status"`
	HasValue              bool                        `json:"has_value"`
	ActiveVersion         int                         `json:"active_version"`
	ExpiresAt             *time.Time                  `json:"expires_at,omitempty"`
	LastRotatedAt         *time.Time                  `json:"last_rotated_at,omitempty"`
	ManagedByConfigRepo   bool                        `json:"managed_by_config_repo"`
	ConfigRepoID          *int64                      `json:"config_repo_id,omitempty"`
	ConfigSourcePath      string                      `json:"config_source_path,omitempty"`
	ConfigSourceCommitSHA string                      `json:"config_source_commit_sha,omitempty"`
	CreatedBy             string                      `json:"created_by,omitempty"`
	UpdatedBy             string                      `json:"updated_by,omitempty"`
	CreatedAt             time.Time                   `json:"created_at"`
	UpdatedAt             time.Time                   `json:"updated_at"`
	Versions              []credentialVersionResponse `json:"versions,omitempty"`
}

type credentialVersionResponse struct {
	Version     int        `json:"version"`
	CreatedBy   string     `json:"created_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ActivatedAt *time.Time `json:"activated_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

type credentialsResponse struct {
	Credentials []credentialResponse `json:"credentials"`
}

func credentialResponseFromRecord(record credentials.Credential) credentialResponse {
	return credentialResponse{
		ID:                    record.ID.String(),
		Reference:             record.Reference.String(),
		Kind:                  record.Kind,
		Description:           record.Description,
		Status:                record.Status,
		HasValue:              record.HasValue(),
		ActiveVersion:         record.ActiveVersion,
		ExpiresAt:             record.ExpiresAt,
		LastRotatedAt:         record.LastRotatedAt,
		ManagedByConfigRepo:   record.ManagedByConfigRepo,
		ConfigRepoID:          record.ConfigRepoID,
		ConfigSourcePath:      record.ConfigSourcePath,
		ConfigSourceCommitSHA: record.ConfigSourceCommitSHA,
		CreatedBy:             record.CreatedBy,
		UpdatedBy:             record.UpdatedBy,
		CreatedAt:             record.CreatedAt,
		UpdatedAt:             record.UpdatedAt,
	}
}

func credentialVersionResponses(records []credentials.Version) []credentialVersionResponse {
	response := make([]credentialVersionResponse, 0, len(records))
	for _, record := range records {
		response = append(response, credentialVersionResponse{
			Version:     record.Version,
			CreatedBy:   record.CreatedBy,
			CreatedAt:   record.CreatedAt,
			ActivatedAt: record.ActivatedAt,
			RevokedAt:   record.RevokedAt,
		})
	}
	return response
}
