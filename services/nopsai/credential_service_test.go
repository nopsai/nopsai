package nopsai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"nopsai/config"
	"nopsai/services/nopsai/internal/credentials"
)

type memoryCredentialStore struct {
	credentials map[uuid.UUID]credentials.Credential
	references  map[string]uuid.UUID
	versions    map[uuid.UUID]map[int]credentials.Version
	nextVersion map[uuid.UUID]int
	accesses    []credentials.AccessRecord
}

func newMemoryCredentialStore() *memoryCredentialStore {
	return &memoryCredentialStore{
		credentials: map[uuid.UUID]credentials.Credential{},
		references:  map[string]uuid.UUID{},
		versions:    map[uuid.UUID]map[int]credentials.Version{},
		nextVersion: map[uuid.UUID]int{},
	}
}

func (s *memoryCredentialStore) CreateCredential(_ context.Context, credential credentials.Credential) (credentials.Credential, error) {
	if _, exists := s.references[credential.Reference.String()]; exists {
		return credentials.Credential{}, errors.New("duplicate credential")
	}
	now := time.Now().UTC()
	credential.Status = credentials.StatusPending
	credential.CreatedAt = now
	credential.UpdatedAt = now
	s.credentials[credential.ID] = credential
	s.references[credential.Reference.String()] = credential.ID
	s.nextVersion[credential.ID] = 1
	return credential, nil
}

func (s *memoryCredentialStore) UpsertCredentialMetadata(ctx context.Context, credential credentials.Credential) (credentials.Credential, error) {
	if existingID, ok := s.references[credential.Reference.String()]; ok {
		existing := s.credentials[existingID]
		existing.Kind = credential.Kind
		existing.Description = credential.Description
		existing.ManagedByConfigRepo = credential.ManagedByConfigRepo
		existing.ConfigRepoID = credential.ConfigRepoID
		existing.ConfigSourcePath = credential.ConfigSourcePath
		existing.ConfigSourceCommitSHA = credential.ConfigSourceCommitSHA
		existing.UpdatedBy = credential.UpdatedBy
		s.credentials[existingID] = existing
		return existing, nil
	}
	return s.CreateCredential(ctx, credential)
}

func (s *memoryCredentialStore) GetCredentialByID(_ context.Context, id uuid.UUID) (credentials.Credential, error) {
	credential, ok := s.credentials[id]
	if !ok {
		return credentials.Credential{}, credentials.ErrNotFound
	}
	return credential, nil
}

func (s *memoryCredentialStore) GetCredentialByReference(ctx context.Context, ref credentials.Reference) (credentials.Credential, error) {
	id, ok := s.references[ref.String()]
	if !ok {
		return credentials.Credential{}, credentials.ErrNotFound
	}
	return s.GetCredentialByID(ctx, id)
}

func (s *memoryCredentialStore) ListCredentials(_ context.Context) ([]credentials.Credential, error) {
	result := make([]credentials.Credential, 0, len(s.credentials))
	for _, credential := range s.credentials {
		result = append(result, credential)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Reference.String() < result[j].Reference.String()
	})
	return result, nil
}

func (s *memoryCredentialStore) ListCredentialVersions(_ context.Context, credentialID uuid.UUID) ([]credentials.Version, error) {
	result := make([]credentials.Version, 0, len(s.versions[credentialID]))
	for _, version := range s.versions[credentialID] {
		result = append(result, version)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version > result[j].Version })
	return result, nil
}

func (s *memoryCredentialStore) ReserveCredentialVersion(_ context.Context, credentialID uuid.UUID) (int, error) {
	if _, ok := s.credentials[credentialID]; !ok {
		return 0, credentials.ErrNotFound
	}
	version := s.nextVersion[credentialID]
	s.nextVersion[credentialID] = version + 1
	return version, nil
}

func (s *memoryCredentialStore) CreateCredentialVersion(
	_ context.Context,
	credentialID uuid.UUID,
	version int,
	envelope credentials.Envelope,
	actor string,
	activate bool,
) (credentials.Version, error) {
	if _, ok := s.credentials[credentialID]; !ok {
		return credentials.Version{}, credentials.ErrNotFound
	}
	if s.versions[credentialID] == nil {
		s.versions[credentialID] = map[int]credentials.Version{}
	}
	now := time.Now().UTC()
	record := credentials.Version{
		CredentialID: credentialID,
		Version:      version,
		Envelope:     envelope,
		CreatedBy:    actor,
		CreatedAt:    now,
	}
	if activate {
		record.ActivatedAt = &now
		credential := s.credentials[credentialID]
		credential.Status = credentials.StatusActive
		credential.ActiveVersion = version
		credential.LastRotatedAt = &now
		s.credentials[credentialID] = credential
	}
	s.versions[credentialID][version] = record
	return record, nil
}

func (s *memoryCredentialStore) ActivateCredentialVersion(_ context.Context, credentialID uuid.UUID, version int, actor string) error {
	record, ok := s.versions[credentialID][version]
	if !ok {
		return credentials.ErrNotFound
	}
	now := time.Now().UTC()
	record.ActivatedAt = &now
	record.RevokedAt = nil
	s.versions[credentialID][version] = record
	credential := s.credentials[credentialID]
	credential.Status = credentials.StatusActive
	credential.ActiveVersion = version
	credential.UpdatedBy = actor
	s.credentials[credentialID] = credential
	return nil
}

func (s *memoryCredentialStore) DisableCredential(_ context.Context, credentialID uuid.UUID, actor string) error {
	credential, ok := s.credentials[credentialID]
	if !ok {
		return credentials.ErrNotFound
	}
	credential.Status = credentials.StatusDisabled
	credential.UpdatedBy = actor
	s.credentials[credentialID] = credential
	return nil
}

func (s *memoryCredentialStore) EnableCredential(_ context.Context, credentialID uuid.UUID, actor string) error {
	credential, ok := s.credentials[credentialID]
	if !ok {
		return credentials.ErrNotFound
	}
	if credential.ActiveVersion > 0 {
		credential.Status = credentials.StatusActive
	} else {
		credential.Status = credentials.StatusPending
	}
	credential.UpdatedBy = actor
	s.credentials[credentialID] = credential
	return nil
}

func (s *memoryCredentialStore) DeleteCredentialVersion(_ context.Context, credentialID uuid.UUID, version int) error {
	credential, ok := s.credentials[credentialID]
	if !ok {
		return credentials.ErrNotFound
	}
	records := s.versions[credentialID]
	if _, ok := records[version]; !ok {
		return credentials.ErrNotFound
	}
	if version == credential.ActiveVersion {
		return credentials.ErrActiveVersion
	}
	if len(records) < 2 {
		return credentials.ErrLastVersion
	}
	delete(records, version)
	return nil
}

func (s *memoryCredentialStore) DeleteCredential(_ context.Context, credentialID uuid.UUID) error {
	credential, ok := s.credentials[credentialID]
	if !ok {
		return credentials.ErrNotFound
	}
	delete(s.references, credential.Reference.String())
	delete(s.credentials, credentialID)
	delete(s.versions, credentialID)
	return nil
}

func (s *memoryCredentialStore) ResolveActiveCredential(_ context.Context, ref credentials.Reference) (credentials.ResolvedRecord, error) {
	id, ok := s.references[ref.String()]
	if !ok {
		return credentials.ResolvedRecord{}, credentials.ErrNotFound
	}
	credential := s.credentials[id]
	version, ok := s.versions[id][credential.ActiveVersion]
	if !ok {
		return credentials.ResolvedRecord{}, credentials.ErrUnavailable
	}
	return credentials.ResolvedRecord{Credential: credential, Version: version}, nil
}

func (s *memoryCredentialStore) RecordCredentialAccess(_ context.Context, record credentials.AccessRecord) error {
	s.accesses = append(s.accesses, record)
	return nil
}

func TestCredentialServiceLifecycle(t *testing.T) {
	ctx := context.Background()
	store := newMemoryCredentialStore()
	codec, err := credentials.NewEnvelopeCodec("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("NewEnvelopeCodec() error = %v", err)
	}
	service, err := newCredentialService(store, codec, nil)
	if err != nil {
		t.Fatalf("newCredentialService() error = %v", err)
	}
	ref, _ := credentials.ParseReference("credential://system/llm/openai-primary")
	credential, err := service.Create(ctx, createCredentialInput{
		Reference: ref,
		Kind:      "api_key",
		Value:     []byte("first-secret"),
		Actor:     "admin",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if credential.Status != credentials.StatusActive || credential.ActiveVersion != 1 {
		t.Fatalf("credential = %#v, want active version 1", credential)
	}
	firstVersion := store.versions[credential.ID][1]
	if bytes.Contains(firstVersion.Envelope.Ciphertext, []byte("first-secret")) {
		t.Fatal("encrypted version contains plaintext")
	}

	purpose := credentials.Purpose{ConsumerService: "nopsai", Operation: "llm.authenticate"}
	value, err := service.Resolve(ctx, ref, purpose)
	if err != nil || value.Text() != "first-secret" {
		t.Fatalf("Resolve() = %q, %v", value.Text(), err)
	}
	if len(store.accesses) != 1 || !store.accesses[0].Success {
		t.Fatalf("access records = %#v, want successful resolution", store.accesses)
	}

	if _, err := service.PutValue(ctx, credential.ID, []byte("second-secret"), "admin"); err != nil {
		t.Fatalf("PutValue() error = %v", err)
	}
	if err := service.Activate(ctx, credential.ID, 1, "admin"); err != nil {
		t.Fatalf("Activate() rollback error = %v", err)
	}
	value, err = service.Resolve(ctx, ref, purpose)
	if err != nil || value.Text() != "first-secret" {
		t.Fatalf("Resolve() after rollback = %q, %v", value.Text(), err)
	}

	if err := service.Disable(ctx, credential.ID, "admin"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if _, err := service.Resolve(ctx, ref, purpose); !errors.Is(err, credentials.ErrDisabled) {
		t.Fatalf("Resolve() disabled error = %v, want ErrDisabled", err)
	}
	if got := store.accesses[len(store.accesses)-1].ErrorCode; got != "disabled" {
		t.Fatalf("disabled access error code = %q", got)
	}

	if err := service.Enable(ctx, credential.ID, "admin"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	value, err = service.Resolve(ctx, ref, purpose)
	if err != nil || value.Text() != "first-secret" {
		t.Fatalf("Resolve() after enable = %q, %v", value.Text(), err)
	}

	if err := service.DeleteVersion(ctx, credential.ID, 1, "admin"); !errors.Is(err, credentials.ErrActiveVersion) {
		t.Fatalf("DeleteVersion() active error = %v, want ErrActiveVersion", err)
	}
	if err := service.DeleteVersion(ctx, credential.ID, 2, "admin"); err != nil {
		t.Fatalf("DeleteVersion() old version error = %v", err)
	}
	if _, exists := store.versions[credential.ID][2]; exists {
		t.Fatal("DeleteVersion() retained deleted version 2")
	}
}

func TestCredentialServiceRequiresTwoVersionsBeforeDeletingHistory(t *testing.T) {
	ctx := context.Background()
	store := newMemoryCredentialStore()
	codec, _ := credentials.NewEnvelopeCodec("01234567890123456789012345678901")
	service, _ := newCredentialService(store, codec, nil)
	ref, _ := credentials.ParseReference("credential://system/mail/pending")
	credential, err := service.Create(ctx, createCredentialInput{
		Reference: ref,
		Kind:      "password",
		Actor:     "admin",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	version, err := store.ReserveCredentialVersion(ctx, credential.ID)
	if err != nil {
		t.Fatalf("ReserveCredentialVersion() error = %v", err)
	}
	envelope, err := codec.Encrypt(ref, credential.Kind, version, []byte("pending-secret"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if _, err := store.CreateCredentialVersion(ctx, credential.ID, version, envelope, "admin", false); err != nil {
		t.Fatalf("CreateCredentialVersion() error = %v", err)
	}
	if err := service.DeleteVersion(ctx, credential.ID, version, "admin"); !errors.Is(err, credentials.ErrLastVersion) {
		t.Fatalf("DeleteVersion() error = %v, want ErrLastVersion", err)
	}
}

func TestCredentialMetadataAndBoundDelete(t *testing.T) {
	ctx := context.Background()
	store := newMemoryCredentialStore()
	codec, _ := credentials.NewEnvelopeCodec("01234567890123456789012345678901")
	service, _ := newCredentialService(store, codec, nil)
	ref, _ := credentials.ParseReference("credential://system/github/webhook-secret")
	credential, err := service.EnsureMetadata(ctx, createCredentialInput{
		Reference: ref,
		Kind:      "webhook_secret",
		Actor:     "gitops",
	})
	if err != nil {
		t.Fatalf("EnsureMetadata() error = %v", err)
	}
	if credential.Status != credentials.StatusPending || credential.HasValue() {
		t.Fatalf("credential = %#v, want pending metadata", credential)
	}
	configRepoID := int64(42)
	credential, err = service.EnsureMetadata(ctx, createCredentialInput{
		Reference:             ref,
		Kind:                  "webhook_secret",
		Actor:                 "gitops",
		ManagedByConfigRepo:   true,
		ConfigRepoID:          &configRepoID,
		ConfigSourcePath:      "setting/system/github.yaml",
		ConfigSourceCommitSHA: "abc123",
	})
	if err != nil {
		t.Fatalf("EnsureMetadata() ownership error = %v", err)
	}
	if !credential.ManagedByConfigRepo || credential.ConfigRepoID == nil ||
		*credential.ConfigRepoID != configRepoID || credential.ConfigSourceCommitSHA != "abc123" {
		t.Fatalf("credential provenance = %#v, want GitOps ownership", credential)
	}
	if _, err := service.EnsureMetadata(ctx, createCredentialInput{
		Reference: ref,
		Kind:      "password",
	}); err == nil {
		t.Fatal("EnsureMetadata() accepted a conflicting credential kind")
	}

	app := &App{
		cfg:             &config.Config{GitHubWebhookCredentialRef: ref.String()},
		credentialStore: store,
		credentials:     service,
	}
	request := httptest.NewRequest(http.MethodDelete, "/v1/system/credentials/"+credential.ID.String(), nil)
	request.SetPathValue("credentialID", credential.ID.String())
	response := httptest.NewRecorder()
	app.handleDeleteCredential(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("delete status = %d, want %d", response.Code, http.StatusConflict)
	}
	if _, err := store.GetCredentialByID(ctx, credential.ID); err != nil {
		t.Fatalf("bound credential was deleted: %v", err)
	}
}

func TestLegacyCredentialMigrationHelpersImportAndScrubValues(t *testing.T) {
	ctx := context.Background()
	store := newMemoryCredentialStore()
	codec, _ := credentials.NewEnvelopeCodec("01234567890123456789012345678901")
	service, _ := newCredentialService(store, codec, nil)
	importer := credentialImporter{service: service, store: store}

	t.Setenv("LEGACY_LLM_KEY", "legacy-secret")
	ref, err := migrateEnvironmentCredentialReference(
		ctx,
		importer,
		"",
		"LEGACY_LLM_KEY",
		"credential://system/llm/legacy",
		"api_key",
		"Legacy LLM key",
	)
	if err != nil {
		t.Fatalf("migrateEnvironmentCredentialReference() error = %v", err)
	}
	parsedRef, _ := credentials.ParseReference(ref)
	value, err := service.Resolve(ctx, parsedRef, credentials.Purpose{
		ConsumerService: "nopsai",
		Operation:       "llm.authenticate",
	})
	if err != nil || value.Text() != "legacy-secret" {
		t.Fatalf("migrated LLM value = %q, %v", value.Text(), err)
	}

	clientRef, entitlementJSON, err := migrateOIDCProviderCredentialValues(
		ctx,
		importer,
		"corporate",
		"",
		"oidc-client-secret",
		[]byte(`{"admin_client_secret":"admin-client-secret","admin_password":"admin-password"}`),
	)
	if err != nil {
		t.Fatalf("migrateOIDCProviderCredentialValues() error = %v", err)
	}
	if clientRef != "credential://system/oidc/corporate/client-secret" {
		t.Fatalf("client reference = %q", clientRef)
	}
	var entitlement map[string]any
	if err := json.Unmarshal(entitlementJSON, &entitlement); err != nil {
		t.Fatalf("unmarshal entitlement: %v", err)
	}
	if _, exists := entitlement["admin_client_secret"]; exists {
		t.Fatal("legacy admin client secret was not scrubbed")
	}
	if _, exists := entitlement["admin_password"]; exists {
		t.Fatal("legacy admin password was not scrubbed")
	}
	if entitlement["admin_client_credential_ref"] != "credential://system/oidc/corporate/admin-client-secret" ||
		entitlement["admin_password_credential_ref"] != "credential://system/oidc/corporate/admin-password" {
		t.Fatalf("migrated entitlement references = %#v", entitlement)
	}
}
