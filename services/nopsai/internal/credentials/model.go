package credentials

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusPending  = "pending"
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

type Credential struct {
	ID                    uuid.UUID
	Reference             Reference
	Kind                  string
	Description           string
	Status                string
	ActiveVersion         int
	ExpiresAt             *time.Time
	LastRotatedAt         *time.Time
	ManagedByConfigRepo   bool
	ConfigRepoID          *int64
	ConfigSourcePath      string
	ConfigSourceCommitSHA string
	CreatedBy             string
	UpdatedBy             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (c Credential) HasValue() bool {
	return c.ActiveVersion > 0 && c.Status == StatusActive
}

type Version struct {
	CredentialID uuid.UUID
	Version      int
	Envelope     Envelope
	CreatedBy    string
	CreatedAt    time.Time
	ActivatedAt  *time.Time
	RevokedAt    *time.Time
}

type ResolvedRecord struct {
	Credential Credential
	Version    Version
}

type AccessRecord struct {
	CredentialID    uuid.UUID
	Version         int
	ConsumerService string
	Purpose         string
	SubjectType     string
	SubjectID       string
	CorrelationID   string
	Success         bool
	ErrorCode       string
	CreatedAt       time.Time
}
