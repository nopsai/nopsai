package nopsai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"nopsai/pkg/registryauth"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/pkg/audit"
	"nopsai/services/nopsai/pkg/store"
)

type CredentialResolver = credentials.Resolver

type credentialService struct {
	store store.CredentialStore
	codec *credentials.EnvelopeCodec
	audit *audit.Logger
}

type createCredentialInput struct {
	Reference             credentials.Reference
	Kind                  string
	Description           string
	ExpiresAt             *time.Time
	Value                 []byte
	Actor                 string
	ManagedByConfigRepo   bool
	ConfigRepoID          *int64
	ConfigSourcePath      string
	ConfigSourceCommitSHA string
}

func newCredentialService(
	credentialStore store.CredentialStore,
	codec *credentials.EnvelopeCodec,
	auditLogger *audit.Logger,
) (*credentialService, error) {
	if credentialStore == nil {
		return nil, errors.New("credential store is required")
	}
	if codec == nil {
		return nil, errors.New("credential codec is required")
	}
	return &credentialService{store: credentialStore, codec: codec, audit: auditLogger}, nil
}

func (s *credentialService) Create(ctx context.Context, input createCredentialInput) (credentials.Credential, error) {
	if input.Reference.String() == "" {
		return credentials.Credential{}, credentials.ErrInvalidReference
	}
	kind, err := normalizeCredentialKind(input.Kind)
	if err != nil {
		return credentials.Credential{}, err
	}
	actor := strings.TrimSpace(input.Actor)
	credential, err := s.store.CreateCredential(ctx, credentials.Credential{
		ID:                    uuid.New(),
		Reference:             input.Reference,
		Kind:                  kind,
		Description:           strings.TrimSpace(input.Description),
		ExpiresAt:             input.ExpiresAt,
		ManagedByConfigRepo:   input.ManagedByConfigRepo,
		ConfigRepoID:          input.ConfigRepoID,
		ConfigSourcePath:      strings.TrimSpace(input.ConfigSourcePath),
		ConfigSourceCommitSHA: strings.TrimSpace(input.ConfigSourceCommitSHA),
		CreatedBy:             actor,
		UpdatedBy:             actor,
	})
	if err != nil {
		return credentials.Credential{}, err
	}
	if len(input.Value) == 0 {
		s.auditManagement(ctx, actor, "credential.create", credential, "success", nil)
		return credential, nil
	}
	if _, err := s.PutValue(ctx, credential.ID, input.Value, actor); err != nil {
		_ = s.store.DeleteCredential(ctx, credential.ID)
		return credentials.Credential{}, err
	}
	credential, err = s.store.GetCredentialByID(ctx, credential.ID)
	if err == nil {
		s.auditManagement(ctx, actor, "credential.create", credential, "success", nil)
	}
	return credential, err
}

func (s *credentialService) EnsureMetadata(ctx context.Context, input createCredentialInput) (credentials.Credential, error) {
	if input.Reference.String() == "" {
		return credentials.Credential{}, credentials.ErrInvalidReference
	}
	kind, err := normalizeCredentialKind(input.Kind)
	if err != nil {
		return credentials.Credential{}, err
	}
	existing, err := s.store.GetCredentialByReference(ctx, input.Reference)
	if err == nil {
		if existing.Kind != kind {
			return credentials.Credential{}, fmt.Errorf(
				"credential %s already exists with kind %q; expected kind %q",
				input.Reference.String(),
				existing.Kind,
				kind,
			)
		}
		if shouldRefreshManagedCredentialMetadata(existing, input) {
			input.Reference = existing.Reference
			input.Kind = existing.Kind
			return s.store.UpsertCredentialMetadata(ctx, credentials.Credential{
				ID:                    existing.ID,
				Reference:             input.Reference,
				Kind:                  input.Kind,
				Description:           strings.TrimSpace(input.Description),
				ExpiresAt:             input.ExpiresAt,
				ManagedByConfigRepo:   true,
				ConfigRepoID:          input.ConfigRepoID,
				ConfigSourcePath:      strings.TrimSpace(input.ConfigSourcePath),
				ConfigSourceCommitSHA: strings.TrimSpace(input.ConfigSourceCommitSHA),
				CreatedBy:             existing.CreatedBy,
				UpdatedBy:             strings.TrimSpace(input.Actor),
			})
		}
		return existing, nil
	}
	if !errors.Is(err, credentials.ErrNotFound) {
		return credentials.Credential{}, err
	}
	actor := strings.TrimSpace(input.Actor)
	return s.store.UpsertCredentialMetadata(ctx, credentials.Credential{
		ID:                    uuid.New(),
		Reference:             input.Reference,
		Kind:                  kind,
		Description:           strings.TrimSpace(input.Description),
		ExpiresAt:             input.ExpiresAt,
		ManagedByConfigRepo:   input.ManagedByConfigRepo,
		ConfigRepoID:          input.ConfigRepoID,
		ConfigSourcePath:      strings.TrimSpace(input.ConfigSourcePath),
		ConfigSourceCommitSHA: strings.TrimSpace(input.ConfigSourceCommitSHA),
		CreatedBy:             actor,
		UpdatedBy:             actor,
	})
}

func shouldRefreshManagedCredentialMetadata(existing credentials.Credential, input createCredentialInput) bool {
	if !input.ManagedByConfigRepo || !existing.ManagedByConfigRepo {
		return false
	}
	if existing.ConfigRepoID == nil || input.ConfigRepoID == nil {
		return existing.ConfigRepoID == nil && input.ConfigRepoID == nil
	}
	return *existing.ConfigRepoID == *input.ConfigRepoID
}

func (s *credentialService) PutValue(
	ctx context.Context,
	credentialID uuid.UUID,
	plaintext []byte,
	actor string,
) (credentials.Version, error) {
	if len(plaintext) == 0 {
		return credentials.Version{}, errors.New("credential value is required")
	}
	credential, err := s.store.GetCredentialByID(ctx, credentialID)
	if err != nil {
		return credentials.Version{}, err
	}
	metadata, err := credentialMetadataForValue(credential.Kind, plaintext)
	if err != nil {
		return credentials.Version{}, err
	}
	version, err := s.store.ReserveCredentialVersion(ctx, credentialID)
	if err != nil {
		return credentials.Version{}, err
	}
	envelope, err := s.codec.Encrypt(credential.Reference, credential.Kind, version, plaintext)
	if err != nil {
		return credentials.Version{}, err
	}
	created, err := s.store.CreateCredentialVersion(ctx, credentialID, version, envelope, actor, true)
	if err != nil {
		return credentials.Version{}, err
	}
	if err := s.store.UpdateCredentialMetadata(ctx, credentialID, metadata, actor); err != nil {
		return credentials.Version{}, err
	}
	credential.ActiveVersion = version
	credential.Status = credentials.StatusActive
	credential.Metadata = metadata
	s.auditManagement(ctx, actor, "credential.rotate", credential, "success", map[string]any{"version": version})
	return created, nil
}

func (s *credentialService) Activate(ctx context.Context, credentialID uuid.UUID, version int, actor string) error {
	if version <= 0 {
		return errors.New("credential version must be positive")
	}
	if err := s.store.ActivateCredentialVersion(ctx, credentialID, version, actor); err != nil {
		return err
	}
	credential, _ := s.store.GetCredentialByID(ctx, credentialID)
	s.auditManagement(ctx, actor, "credential.activate", credential, "success", map[string]any{"version": version})
	return nil
}

func (s *credentialService) Disable(ctx context.Context, credentialID uuid.UUID, actor string) error {
	if err := s.store.DisableCredential(ctx, credentialID, actor); err != nil {
		return err
	}
	credential, _ := s.store.GetCredentialByID(ctx, credentialID)
	s.auditManagement(ctx, actor, "credential.disable", credential, "success", nil)
	return nil
}

func (s *credentialService) Enable(ctx context.Context, credentialID uuid.UUID, actor string) error {
	if err := s.store.EnableCredential(ctx, credentialID, actor); err != nil {
		return err
	}
	credential, _ := s.store.GetCredentialByID(ctx, credentialID)
	s.auditManagement(ctx, actor, "credential.enable", credential, "success", nil)
	return nil
}

func (s *credentialService) DeleteVersion(
	ctx context.Context,
	credentialID uuid.UUID,
	version int,
	actor string,
) error {
	if version <= 0 {
		return errors.New("credential version must be positive")
	}
	credential, err := s.store.GetCredentialByID(ctx, credentialID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteCredentialVersion(ctx, credentialID, version); err != nil {
		return err
	}
	s.auditManagement(ctx, actor, "credential.delete_version", credential, "success", map[string]any{
		"version": version,
	})
	return nil
}

func (s *credentialService) Delete(ctx context.Context, credentialID uuid.UUID, actor string) error {
	credential, err := s.store.GetCredentialByID(ctx, credentialID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteCredential(ctx, credentialID); err != nil {
		return err
	}
	s.auditManagement(ctx, actor, "credential.delete", credential, "success", nil)
	return nil
}

func (s *credentialService) Resolve(
	ctx context.Context,
	ref credentials.Reference,
	purpose credentials.Purpose,
) (credentials.Value, error) {
	purpose, err := purpose.Normalize()
	if err != nil {
		return credentials.Value{}, err
	}
	record, err := s.store.ResolveActiveCredential(ctx, ref)
	if err != nil {
		return credentials.Value{}, err
	}
	if record.Credential.Status == credentials.StatusDisabled {
		s.recordAccess(ctx, record, purpose, false, "disabled")
		return credentials.Value{}, credentials.ErrDisabled
	}
	if record.Credential.Status != credentials.StatusActive || record.Credential.ActiveVersion <= 0 {
		s.recordAccess(ctx, record, purpose, false, "unavailable")
		return credentials.Value{}, credentials.ErrUnavailable
	}
	if record.Credential.ExpiresAt != nil && !record.Credential.ExpiresAt.After(time.Now()) {
		s.recordAccess(ctx, record, purpose, false, "expired")
		return credentials.Value{}, credentials.ErrExpired
	}
	if record.Version.RevokedAt != nil {
		s.recordAccess(ctx, record, purpose, false, "revoked")
		return credentials.Value{}, credentials.ErrUnavailable
	}
	plaintext, err := s.codec.Decrypt(
		record.Credential.Reference,
		record.Credential.Kind,
		record.Version.Version,
		record.Version.Envelope,
	)
	if err != nil {
		s.recordAccess(ctx, record, purpose, false, "decrypt_failed")
		return credentials.Value{}, err
	}
	s.recordAccess(ctx, record, purpose, true, "")
	return credentials.NewValue(record.Credential.ID, record.Version.Version, plaintext), nil
}

func (s *credentialService) recordAccess(
	ctx context.Context,
	record credentials.ResolvedRecord,
	purpose credentials.Purpose,
	success bool,
	errorCode string,
) {
	_ = s.store.RecordCredentialAccess(ctx, credentials.AccessRecord{
		CredentialID:    record.Credential.ID,
		Version:         record.Version.Version,
		ConsumerService: purpose.ConsumerService,
		Purpose:         purpose.Operation,
		SubjectType:     purpose.SubjectType,
		SubjectID:       purpose.SubjectID,
		CorrelationID:   purpose.CorrelationID,
		Success:         success,
		ErrorCode:       errorCode,
	})
	if s.audit != nil {
		result := "success"
		if !success {
			result = "failure"
		}
		_ = s.audit.Write(ctx, audit.Entry{
			ActorSub: purpose.SubjectID,
			Provider: purpose.SubjectType,
			Action:   "credential.use",
			Resource: record.Credential.Reference.ResourceID(),
			Result:   result,
			Metadata: map[string]any{
				"credential_id":    record.Credential.ID.String(),
				"version":          record.Version.Version,
				"consumer_service": purpose.ConsumerService,
				"purpose":          purpose.Operation,
				"correlation_id":   purpose.CorrelationID,
				"error_code":       errorCode,
			},
		})
	}
}

func (s *credentialService) auditManagement(
	ctx context.Context,
	actor, action string,
	credential credentials.Credential,
	result string,
	metadata map[string]any,
) {
	if s.audit == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["credential_id"] = credential.ID.String()
	metadata["kind"] = credential.Kind
	_ = s.audit.Write(ctx, audit.Entry{
		ActorSub: actor,
		Action:   action,
		Resource: credential.Reference.ResourceID(),
		Result:   result,
		Metadata: metadata,
	})
}

func normalizeCredentialKind(raw string) (string, error) {
	kind := strings.ToLower(strings.TrimSpace(raw))
	switch kind {
	case "api_key", "password", "bearer_token", "private_key", "webhook_secret", "client_secret",
		registryauth.CredentialKindDockerConfigJSON:
		return kind, nil
	default:
		return "", fmt.Errorf("unsupported credential kind %q", raw)
	}
}

func credentialMetadataForValue(kind string, plaintext []byte) (map[string]any, error) {
	switch kind {
	case registryauth.CredentialKindDockerConfigJSON:
		hosts, err := registryauth.RegistryHosts(plaintext)
		if err != nil {
			return nil, err
		}
		return map[string]any{"registry_hosts": hosts}, nil
	default:
		return map[string]any{}, nil
	}
}

func (a *App) resolveCredentialText(ctx context.Context, rawReference string, purpose credentials.Purpose) (string, error) {
	rawReference = strings.TrimSpace(rawReference)
	if rawReference == "" {
		return "", nil
	}
	if a == nil || a.credentialResolver == nil {
		return "", credentials.ErrUnavailable
	}
	ref, err := credentials.ParseReference(rawReference)
	if err != nil {
		return "", err
	}
	value, err := a.credentialResolver.Resolve(ctx, ref, purpose)
	if err != nil {
		return "", err
	}
	return value.Text(), nil
}
