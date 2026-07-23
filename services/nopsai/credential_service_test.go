package nopsai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"nopsai/config"
	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/pkg/auth"
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
	existingID := credential.ID
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
	if credential.ID != existingID || credential.ManagedByConfigRepo || credential.ConfigRepoID != nil {
		t.Fatalf("credential provenance = %#v, want existing manual credential unchanged", credential)
	}
	if _, err := service.EnsureMetadata(ctx, createCredentialInput{
		Reference: ref,
		Kind:      "password",
	}); err == nil {
		t.Fatal("EnsureMetadata() accepted a conflicting credential kind")
	} else if !strings.Contains(err.Error(), `kind "webhook_secret"`) || !strings.Contains(err.Error(), `expected kind "password"`) {
		t.Fatalf("EnsureMetadata() conflict error = %q, want existing and expected kinds", err.Error())
	}

	managedRef, _ := credentials.ParseReference("credential://system/mcp/github")
	managed, err := service.EnsureMetadata(ctx, createCredentialInput{
		Reference:             managedRef,
		Kind:                  "bearer_token",
		Actor:                 "gitops",
		ManagedByConfigRepo:   true,
		ConfigRepoID:          &configRepoID,
		ConfigSourcePath:      "setting/system/mcp.yaml",
		ConfigSourceCommitSHA: "abc123",
	})
	if err != nil {
		t.Fatalf("EnsureMetadata() managed create error = %v", err)
	}
	if !managed.ManagedByConfigRepo || managed.ConfigRepoID == nil ||
		*managed.ConfigRepoID != configRepoID || managed.ConfigSourceCommitSHA != "abc123" {
		t.Fatalf("managed credential provenance = %#v, want GitOps ownership", managed)
	}
	managed, err = service.EnsureMetadata(ctx, createCredentialInput{
		Reference:             managedRef,
		Kind:                  "bearer_token",
		Description:           "updated MCP token",
		Actor:                 "gitops",
		ManagedByConfigRepo:   true,
		ConfigRepoID:          &configRepoID,
		ConfigSourcePath:      "setting/system/mcp.yaml",
		ConfigSourceCommitSHA: "def456",
	})
	if err != nil {
		t.Fatalf("EnsureMetadata() managed refresh error = %v", err)
	}
	if managed.Description != "updated MCP token" || managed.ConfigSourceCommitSHA != "def456" {
		t.Fatalf("managed credential metadata = %#v, want refreshed GitOps metadata", managed)
	}

	app := &App{
		cfg:             &config.Config{GitHubWebhookCredentialRef: ref.String()},
		credentialStore: store,
		credentials:     service,
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, _ aaamodel.Subject, action string, resource aaamodel.ResourceRef, _ map[string]any) (aaamodel.Decision, error) {
				allowed := action == "iam.admin" && resource.Type == "iam" && resource.ID == "admin"
				return aaamodel.Decision{Allowed: allowed}, nil
			},
		},
	}
	request := credentialTestRequest(http.MethodDelete, "/v1/system/credentials/"+credential.ID.String(), "gitops", nil)
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

func TestCredentialHandlersFilterMetadataToAdminOrTeamScopedRecords(t *testing.T) {
	store := newMemoryCredentialStore()
	own := mustStoreCredential(t, store, "credential://system/llm/alice", "alice")
	shared := mustStoreCredential(t, store, "credential://system/mcp/shared", "bob")
	teamScoped := mustStoreCredential(t, store, "credential://team/platform/llm/shared", "bob")
	other := mustStoreCredential(t, store, "credential://system/llm/bob", "bob")

	app := &App{
		credentialStore: store,
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, subject aaamodel.Subject, action string, resource aaamodel.ResourceRef, _ map[string]any) (aaamodel.Decision, error) {
				if subject.Sub == "admin" && action == "iam.admin" && resource.Type == "iam" && resource.ID == "admin" {
					return aaamodel.Decision{Allowed: true}, nil
				}
				if action == "team.read" && resource.Type == grantResourceTeam && resource.ID == "platform" {
					return aaamodel.Decision{Allowed: true}, nil
				}
				return aaamodel.Decision{Allowed: false}, nil
			},
		},
	}

	req := credentialTestRequest(http.MethodGet, "/v1/system/credentials", "alice", nil)
	rec := httptest.NewRecorder()
	app.handleListCredentials(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response credentialsResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	got := make([]string, 0, len(response.Credentials))
	for _, credential := range response.Credentials {
		got = append(got, credential.Reference)
	}
	sort.Strings(got)
	want := []string{teamScoped.Reference.String()}
	sort.Strings(want)
	if !equalStringSlices(got, want) {
		t.Fatalf("credentials = %#v, want %#v; hidden credential was %s", got, want, other.Reference.String())
	}

	req = credentialTestRequest(http.MethodGet, "/v1/system/credentials", "admin", nil)
	rec = httptest.NewRecorder()
	app.handleListCredentials(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	response = credentialsResponse{}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode admin response: %v", err)
	}
	got = got[:0]
	for _, credential := range response.Credentials {
		got = append(got, credential.Reference)
	}
	sort.Strings(got)
	want = []string{own.Reference.String(), shared.Reference.String(), teamScoped.Reference.String(), other.Reference.String()}
	sort.Strings(want)
	if !equalStringSlices(got, want) {
		t.Fatalf("admin credentials = %#v, want %#v", got, want)
	}
}

func TestCredentialHandlersAllowAdminAndTeamDetailsOnly(t *testing.T) {
	store := newMemoryCredentialStore()
	own := mustStoreCredential(t, store, "credential://system/llm/alice", "alice")
	teamScoped := mustStoreCredential(t, store, "credential://team/platform/llm/shared", "bob")
	other := mustStoreCredential(t, store, "credential://system/llm/bob", "bob")
	app := &App{
		credentialStore: store,
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, subject aaamodel.Subject, action string, resource aaamodel.ResourceRef, _ map[string]any) (aaamodel.Decision, error) {
				if subject.Sub == "admin" && action == "iam.admin" && resource.Type == "iam" && resource.ID == "admin" {
					return aaamodel.Decision{Allowed: true}, nil
				}
				if action == "team.read" && resource.Type == grantResourceTeam && resource.ID == "platform" {
					return aaamodel.Decision{Allowed: true}, nil
				}
				return aaamodel.Decision{Allowed: false}, nil
			},
		},
	}

	req := credentialTestRequest(http.MethodGet, "/v1/system/credentials/"+own.ID.String(), "alice", nil)
	req.SetPathValue("credentialID", own.ID.String())
	rec := httptest.NewRecorder()
	app.handleGetCredential(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("own system status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	req = credentialTestRequest(http.MethodGet, "/v1/system/credentials/"+teamScoped.ID.String(), "alice", nil)
	req.SetPathValue("credentialID", teamScoped.ID.String())
	rec = httptest.NewRecorder()
	app.handleGetCredential(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("team scoped status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	req = credentialTestRequest(http.MethodGet, "/v1/system/credentials/"+other.ID.String(), "alice", nil)
	req.SetPathValue("credentialID", other.ID.String())
	rec = httptest.NewRecorder()
	app.handleGetCredential(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("other status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	req = credentialTestRequest(http.MethodGet, "/v1/system/credentials/"+other.ID.String(), "admin", nil)
	req.SetPathValue("credentialID", other.ID.String())
	rec = httptest.NewRecorder()
	app.handleGetCredential(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin system status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestCreateSystemCredentialRequiresNopsAIAdmin(t *testing.T) {
	store := newMemoryCredentialStore()
	codec, _ := credentials.NewEnvelopeCodec("01234567890123456789012345678901")
	service, _ := newCredentialService(store, codec, nil)
	app := &App{
		credentialStore: store,
		credentials:     service,
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, subject aaamodel.Subject, action string, resource aaamodel.ResourceRef, _ map[string]any) (aaamodel.Decision, error) {
				allowed := subject.Sub == "alice" && action == "system.update" && resource.Type == "system" && resource.ID == "llm-profiles"
				allowed = allowed || subject.Sub == "admin" && action == "iam.admin" && resource.Type == "iam" && resource.ID == "admin"
				return aaamodel.Decision{Allowed: allowed}, nil
			},
		},
	}

	body := bytes.NewBufferString(`{"reference":"credential://system/llm/alice","kind":"api_key","value":"secret"}`)
	req := credentialTestRequest(http.MethodPost, "/v1/system/credentials", "alice", body)
	rec := httptest.NewRecorder()
	app.handleCreateCredential(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	body = bytes.NewBufferString(`{"reference":"credential://system/llm/alice","kind":"api_key","value":"secret"}`)
	req = credentialTestRequest(http.MethodPost, "/v1/system/credentials", "admin", body)
	rec = httptest.NewRecorder()
	app.handleCreateCredential(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	created, err := store.GetCredentialByReference(context.Background(), credentials.Reference{Namespace: "system", Name: "llm/alice"})
	if err != nil {
		t.Fatalf("created credential not stored: %v", err)
	}
	if created.CreatedBy != "admin" || created.Status != credentials.StatusActive {
		t.Fatalf("created credential = %#v, want active admin-owned credential", created)
	}
}

func TestCreateTeamCredentialRequiresMatchingTeamAccess(t *testing.T) {
	store := newMemoryCredentialStore()
	codec, _ := credentials.NewEnvelopeCodec("01234567890123456789012345678901")
	service, _ := newCredentialService(store, codec, nil)
	app := &App{
		credentialStore: store,
		credentials:     service,
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, _ aaamodel.Subject, action string, resource aaamodel.ResourceRef, _ map[string]any) (aaamodel.Decision, error) {
				allowed := action == "credential.create" && resource.Type == grantResourceTeam && resource.ID == "platform"
				return aaamodel.Decision{Allowed: allowed}, nil
			},
		},
	}

	body := bytes.NewBufferString(`{"reference":"credential://team/platform/llm/alice","team_path":"platform","kind":"api_key","value":"secret"}`)
	req := credentialTestRequest(http.MethodPost, "/v1/system/credentials", "alice", body)
	rec := httptest.NewRecorder()
	app.handleCreateCredential(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	created, err := store.GetCredentialByReference(context.Background(), credentials.Reference{Namespace: "team", Name: "platform/llm/alice"})
	if err != nil {
		t.Fatalf("created team credential not stored: %v", err)
	}
	if created.CreatedBy != "alice" || created.Status != credentials.StatusActive {
		t.Fatalf("created credential = %#v, want active alice-owned team credential", created)
	}
}

func TestTeamCredentialMutationUsesTeamScopedActions(t *testing.T) {
	store := newMemoryCredentialStore()
	codec, _ := credentials.NewEnvelopeCodec("01234567890123456789012345678901")
	service, _ := newCredentialService(store, codec, nil)
	teamScoped := mustStoreCredential(t, store, "credential://team/platform/llm/shared", "bob")
	systemScoped := mustStoreCredential(t, store, "credential://system/llm/shared", "bob")
	app := &App{
		credentialStore: store,
		credentials:     service,
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, _ aaamodel.Subject, action string, resource aaamodel.ResourceRef, _ map[string]any) (aaamodel.Decision, error) {
				if resource.Type == grantResourceTeam && resource.ID == "platform" &&
					(action == "credential.disable" || action == "team.read") {
					return aaamodel.Decision{Allowed: true}, nil
				}
				if resource.Type == "credential" && resource.ID == systemScoped.Reference.ResourceID() && action == "credential.disable" {
					return aaamodel.Decision{Allowed: true}, nil
				}
				return aaamodel.Decision{Allowed: false}, nil
			},
		},
	}

	req := credentialTestRequest(http.MethodPost, "/v1/system/credentials/"+teamScoped.ID.String()+"/disable", "alice", nil)
	req.SetPathValue("credentialID", teamScoped.ID.String())
	rec := httptest.NewRecorder()
	app.handleDisableCredential(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("team disable status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	updated, err := store.GetCredentialByID(context.Background(), teamScoped.ID)
	if err != nil {
		t.Fatalf("load team credential: %v", err)
	}
	if updated.Status != credentials.StatusDisabled {
		t.Fatalf("team credential status = %q, want disabled", updated.Status)
	}

	req = credentialTestRequest(http.MethodPost, "/v1/system/credentials/"+systemScoped.ID.String()+"/disable", "alice", nil)
	req.SetPathValue("credentialID", systemScoped.ID.String())
	rec = httptest.NewRecorder()
	app.handleDisableCredential(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("system disable status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCreateTeamCredentialRejectsMissingOrMismatchedTeamPath(t *testing.T) {
	store := newMemoryCredentialStore()
	codec, _ := credentials.NewEnvelopeCodec("01234567890123456789012345678901")
	service, _ := newCredentialService(store, codec, nil)
	app := &App{
		credentialStore: store,
		credentials:     service,
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, _ aaamodel.Subject, _ string, _ aaamodel.ResourceRef, _ map[string]any) (aaamodel.Decision, error) {
				return aaamodel.Decision{Allowed: true}, nil
			},
		},
	}

	for _, body := range []string{
		`{"reference":"credential://team/platform/llm/alice","kind":"api_key"}`,
		`{"reference":"credential://team/platform/llm/alice","team_path":"other","kind":"api_key"}`,
		`{"reference":"credential://system/llm/alice","team_path":"platform","kind":"api_key"}`,
	} {
		req := credentialTestRequest(http.MethodPost, "/v1/system/credentials", "alice", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		app.handleCreateCredential(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s status = %d, want %d", body, rec.Code, http.StatusBadRequest)
		}
	}
}

func mustStoreCredential(t *testing.T, store *memoryCredentialStore, reference string, createdBy string) credentials.Credential {
	t.Helper()
	ref, err := credentials.ParseReference(reference)
	if err != nil {
		t.Fatalf("ParseReference(%q): %v", reference, err)
	}
	record, err := store.CreateCredential(context.Background(), credentials.Credential{
		ID:          uuid.New(),
		Reference:   ref,
		Kind:        "api_key",
		Description: "test credential",
		CreatedBy:   createdBy,
		UpdatedBy:   createdBy,
	})
	if err != nil {
		t.Fatalf("CreateCredential(%q): %v", reference, err)
	}
	return record
}

func credentialTestRequest(method, path, sub string, body *bytes.Buffer) *http.Request {
	var requestBody io.Reader
	if body != nil {
		requestBody = body
	}
	req := httptest.NewRequest(method, path, requestBody)
	claims := &auth.Claims{Sub: sub, Email: sub + "@example.test", Provider: "local"}
	ctx := auth.WithClaims(req.Context(), claims)
	ctx = withAAASubject(ctx, aaamodel.Subject{Type: aaamodel.SubjectTypeUser, Sub: sub, Email: sub + "@example.test"})
	return req.WithContext(ctx)
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
