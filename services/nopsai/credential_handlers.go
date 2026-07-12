package nopsai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"nopsai/pkg/httpapi"
	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/pkg/auth"
)

func (a *App) handleListCredentials(w http.ResponseWriter, r *http.Request) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return
	}
	records, err := a.credentialStore.ListCredentials(r.Context())
	if err != nil {
		http.Error(w, "failed to list credentials", http.StatusInternalServerError)
		return
	}
	records, err = a.filterVisibleCredentials(r, subject, records)
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	response := credentialsResponse{Credentials: make([]credentialResponse, 0, len(records))}
	for _, record := range records {
		response.Credentials = append(response.Credentials, credentialResponseFromRecord(record))
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) handleCreateCredential(w http.ResponseWriter, r *http.Request) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return
	}
	var request credentialCreateRequest
	if err := httpapi.DecodeJSON(r, &request); err != nil {
		http.Error(w, "invalid credential payload", http.StatusBadRequest)
		return
	}
	ref, err := credentials.ParseReference(request.Reference)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	teamPath := normalizeCredentialTeamPath(request.TeamPath)
	if err := validateCredentialTeamScope(ref, teamPath); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	allowed, err := a.canCreateCredential(r, subject, ref, teamPath)
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	record, err := a.credentials.Create(r.Context(), createCredentialInput{
		Reference:   ref,
		Kind:        request.Kind,
		Description: request.Description,
		ExpiresAt:   request.ExpiresAt,
		Value:       []byte(request.Value),
		Actor:       credentialActor(r),
	})
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, credentialResponseFromRecord(record))
}

func (a *App) handleGetCredential(w http.ResponseWriter, r *http.Request) {
	credentialID, ok := credentialIDFromRequest(w, r)
	if !ok {
		return
	}
	record, err := a.credentialStore.GetCredentialByID(r.Context(), credentialID)
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	allowed, err := a.canReadCredentialMetadata(r, record)
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	versions, err := a.credentialStore.ListCredentialVersions(r.Context(), credentialID)
	if err != nil {
		http.Error(w, "failed to load credential versions", http.StatusInternalServerError)
		return
	}
	response := credentialResponseFromRecord(record)
	response.Versions = credentialVersionResponses(versions)
	writeJSON(w, http.StatusOK, response)
}

func (a *App) handleRotateCredential(w http.ResponseWriter, r *http.Request) {
	credentialID, ok := credentialIDFromRequest(w, r)
	if !ok {
		return
	}
	if !a.requireCredentialRecordAction(w, r, credentialID, "credential.write_value") {
		return
	}
	var request credentialValueRequest
	if err := httpapi.DecodeJSON(r, &request); err != nil {
		http.Error(w, "invalid credential value payload", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(request.Value) == "" {
		http.Error(w, "credential value is required", http.StatusBadRequest)
		return
	}
	if _, err := a.credentials.PutValue(r.Context(), credentialID, []byte(request.Value), credentialActor(r)); err != nil {
		writeCredentialError(w, err)
		return
	}
	a.handleGetCredential(w, r)
}

func (a *App) handleActivateCredentialVersion(w http.ResponseWriter, r *http.Request) {
	credentialID, ok := credentialIDFromRequest(w, r)
	if !ok {
		return
	}
	if !a.requireCredentialRecordAction(w, r, credentialID, "credential.rotate") {
		return
	}
	version, err := strconv.Atoi(strings.TrimSpace(r.PathValue("version")))
	if err != nil || version <= 0 {
		http.Error(w, "credential version must be positive", http.StatusBadRequest)
		return
	}
	if err := a.credentials.Activate(r.Context(), credentialID, version, credentialActor(r)); err != nil {
		writeCredentialError(w, err)
		return
	}
	a.handleGetCredential(w, r)
}

func (a *App) handleDisableCredential(w http.ResponseWriter, r *http.Request) {
	credentialID, ok := credentialIDFromRequest(w, r)
	if !ok {
		return
	}
	if !a.requireCredentialRecordAction(w, r, credentialID, "credential.disable") {
		return
	}
	if err := a.credentials.Disable(r.Context(), credentialID, credentialActor(r)); err != nil {
		writeCredentialError(w, err)
		return
	}
	a.handleGetCredential(w, r)
}

func (a *App) handleEnableCredential(w http.ResponseWriter, r *http.Request) {
	credentialID, ok := credentialIDFromRequest(w, r)
	if !ok {
		return
	}
	if !a.requireCredentialRecordAction(w, r, credentialID, "credential.enable") {
		return
	}
	if err := a.credentials.Enable(r.Context(), credentialID, credentialActor(r)); err != nil {
		writeCredentialError(w, err)
		return
	}
	a.handleGetCredential(w, r)
}

func (a *App) handleDeleteCredentialVersion(w http.ResponseWriter, r *http.Request) {
	credentialID, ok := credentialIDFromRequest(w, r)
	if !ok {
		return
	}
	if !a.requireCredentialRecordAction(w, r, credentialID, "credential.delete_version") {
		return
	}
	version, err := strconv.Atoi(strings.TrimSpace(r.PathValue("version")))
	if err != nil || version <= 0 {
		http.Error(w, "credential version must be positive", http.StatusBadRequest)
		return
	}
	if err := a.credentials.DeleteVersion(r.Context(), credentialID, version, credentialActor(r)); err != nil {
		writeCredentialError(w, err)
		return
	}
	a.handleGetCredential(w, r)
}

func (a *App) handleDeleteCredential(w http.ResponseWriter, r *http.Request) {
	credentialID, ok := credentialIDFromRequest(w, r)
	if !ok {
		return
	}
	record, err := a.credentialStore.GetCredentialByID(r.Context(), credentialID)
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	if !a.requireLoadedCredentialAction(w, r, record, "credential.delete") {
		return
	}
	usages, err := a.credentialReferenceUsages(r.Context(), record.Reference)
	if err != nil {
		http.Error(w, "failed to check credential references", http.StatusInternalServerError)
		return
	}
	if len(usages) > 0 {
		http.Error(
			w,
			"credential is still referenced by "+strings.Join(usages, ", "),
			http.StatusConflict,
		)
		return
	}
	if err := a.credentials.Delete(r.Context(), credentialID, credentialActor(r)); err != nil {
		writeCredentialError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) filterVisibleCredentials(r *http.Request, subject aaamodel.Subject, records []credentials.Credential) ([]credentials.Credential, error) {
	if len(records) == 0 {
		return records, nil
	}
	if allowed, err := a.credentialActionAllowed(r, subject, "credential.list_metadata", aaamodel.ResourceRef{Type: "credential", ID: "*"}); err != nil {
		return nil, err
	} else if allowed {
		return records, nil
	}
	for _, action := range credentialVisibilityActions() {
		if allowed, err := a.credentialActionAllowed(r, subject, action, aaamodel.ResourceRef{Type: "credential", ID: "*"}); err != nil {
			return nil, err
		} else if allowed {
			return records, nil
		}
	}

	resources := make([]aaamodel.ResourceRef, 0, len(records))
	for _, record := range records {
		resources = append(resources, credentialRecordResource(record))
	}
	allowedSet, err := a.allowedResourceSet(r, "credential.list_metadata", resources)
	if err != nil {
		return nil, err
	}
	filtered := make([]credentials.Credential, 0, len(records))
	for _, record := range records {
		resource := credentialRecordResource(record)
		if credentialCreatedBySubject(record, subject) {
			filtered = append(filtered, record)
			continue
		}
		if _, ok := allowedSet[resourceKey(resource)]; ok {
			filtered = append(filtered, record)
			continue
		}
		if allowed, err := a.credentialAnyVisibilityActionAllowed(r, subject, resource); err != nil {
			return nil, err
		} else if allowed {
			filtered = append(filtered, record)
			continue
		}
		if allowed, err := a.credentialTeamScopeVisibilityAllowed(r, subject, record); err != nil {
			return nil, err
		} else if allowed {
			filtered = append(filtered, record)
		}
	}
	return filtered, nil
}

func (a *App) canReadCredentialMetadata(r *http.Request, record credentials.Credential) (bool, error) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		return false, nil
	}
	if credentialCreatedBySubject(record, subject) {
		return true, nil
	}
	resource := credentialRecordResource(record)
	if allowed, err := a.credentialActionAllowed(r, subject, "credential.list_metadata", resource); err != nil || allowed {
		return allowed, err
	}
	if allowed, err := a.credentialAnyVisibilityActionAllowed(r, subject, resource); err != nil || allowed {
		return allowed, err
	}
	return a.credentialTeamScopeVisibilityAllowed(r, subject, record)
}

func (a *App) requireCredentialRecordAction(w http.ResponseWriter, r *http.Request, credentialID uuid.UUID, action string) bool {
	record, err := a.credentialStore.GetCredentialByID(r.Context(), credentialID)
	if err != nil {
		writeCredentialError(w, err)
		return false
	}
	return a.requireLoadedCredentialAction(w, r, record, action)
}

func (a *App) requireLoadedCredentialAction(w http.ResponseWriter, r *http.Request, record credentials.Credential, action string) bool {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return false
	}
	if credentialCreatedBySubject(record, subject) {
		return true
	}
	allowed, err := a.credentialActionAllowed(r, subject, action, credentialRecordResource(record))
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return false
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func (a *App) canCreateCredential(r *http.Request, subject aaamodel.Subject, ref credentials.Reference, teamPath string) (bool, error) {
	if a.checkCapabilityOrScopedGrant(r.Context(), subject, "credential.create", aaamodel.ResourceRef{Type: "credential", ID: "*"}) {
		return true, nil
	}
	if teamPath != "" {
		resource := aaamodel.ResourceRef{Type: grantResourceTeam, ID: teamPath}
		for _, action := range []string{"credential.create", "team.update", "team.manage_acl"} {
			allowed, err := a.credentialActionAllowed(r, subject, action, resource)
			if err != nil {
				return false, err
			}
			if allowed {
				return true, nil
			}
		}
		return false, nil
	}
	if strings.EqualFold(ref.Namespace, "team") {
		return false, nil
	}
	for _, permission := range credentialConsumerWritePermissions() {
		if permission.resource.Type == grantResourceTeam {
			continue
		}
		if a.checkCapabilityOrScopedGrant(r.Context(), subject, permission.action, permission.resource) {
			return true, nil
		}
	}
	return false, nil
}

func validateCredentialTeamScope(ref credentials.Reference, teamPath string) error {
	if strings.EqualFold(ref.Namespace, "team") {
		if teamPath == "" {
			return fmt.Errorf("team_path is required for team credential references")
		}
		if !strings.HasPrefix(strings.Trim(ref.Name, "/"), teamPath+"/") {
			return fmt.Errorf("credential reference must be inside team_path")
		}
		return nil
	}
	if teamPath != "" {
		return fmt.Errorf("team_path requires a credential://team/... reference")
	}
	return nil
}

func normalizeCredentialTeamPath(teamPath string) string {
	normalized := strings.ToLower(strings.Trim(strings.TrimSpace(teamPath), "/"))
	if normalized == "root" || normalized == "global" {
		return ""
	}
	return normalized
}

func (a *App) credentialActionAllowed(r *http.Request, subject aaamodel.Subject, action string, resource aaamodel.ResourceRef) (bool, error) {
	if a == nil || !a.aaaAvailable() {
		return false, fmt.Errorf("authorization unavailable")
	}
	decision, err := a.aaaCheck(r.Context(), subject, action, resource, a.aaaRequestContext(r))
	if err != nil {
		return false, err
	}
	return decision.Allowed, nil
}

func (a *App) credentialAnyVisibilityActionAllowed(r *http.Request, subject aaamodel.Subject, resource aaamodel.ResourceRef) (bool, error) {
	for _, action := range credentialVisibilityActions() {
		allowed, err := a.credentialActionAllowed(r, subject, action, resource)
		if err != nil {
			return false, err
		}
		if allowed {
			return true, nil
		}
	}
	return false, nil
}

func (a *App) credentialTeamScopeVisibilityAllowed(r *http.Request, subject aaamodel.Subject, record credentials.Credential) (bool, error) {
	for _, resource := range credentialTeamScopeResources(record.Reference) {
		for _, action := range []string{"team.read", "team.update", "team.manage_acl", "credential.create", "credential.use"} {
			allowed, err := a.credentialActionAllowed(r, subject, action, resource)
			if err != nil {
				return false, err
			}
			if allowed {
				return true, nil
			}
		}
	}
	return false, nil
}

func credentialTeamScopeResources(ref credentials.Reference) []aaamodel.ResourceRef {
	if !strings.EqualFold(ref.Namespace, "team") {
		return nil
	}
	parts := strings.Split(strings.Trim(ref.Name, "/"), "/")
	if len(parts) < 2 {
		return nil
	}
	resources := make([]aaamodel.ResourceRef, 0, len(parts)-1)
	seen := map[string]struct{}{}
	for i := len(parts) - 1; i >= 1; i-- {
		path := strings.Join(parts[:i], "/")
		if path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		resources = append(resources, aaamodel.ResourceRef{Type: grantResourceTeam, ID: path})
	}
	return resources
}

func credentialVisibilityActions() []string {
	return append([]string{
		"credential.use",
		"credential.manage_acl",
	}, credentialManagementActions()...)
}

func credentialRecordResource(record credentials.Credential) aaamodel.ResourceRef {
	return aaamodel.ResourceRef{Type: "credential", ID: record.Reference.ResourceID()}
}

func credentialCreatedBySubject(record credentials.Credential, subject aaamodel.Subject) bool {
	createdBy := strings.TrimSpace(record.CreatedBy)
	if createdBy == "" {
		return false
	}
	for _, candidate := range credentialSubjectActorIDs(subject) {
		if strings.EqualFold(createdBy, candidate) {
			return true
		}
	}
	return false
}

func credentialSubjectActorIDs(subject aaamodel.Subject) []string {
	candidates := []string{subject.ID, subject.Sub, subject.Email}
	seen := make(map[string]struct{}, len(candidates))
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		key := strings.ToLower(candidate)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

func credentialManagementActions() []string {
	return []string{
		"credential.write_value",
		"credential.rotate",
		"credential.disable",
		"credential.enable",
		"credential.delete_version",
		"credential.delete",
	}
}

func credentialConsumerWritePermissions() []struct {
	action   string
	resource aaamodel.ResourceRef
} {
	return []struct {
		action   string
		resource aaamodel.ResourceRef
	}{
		{action: "system.update", resource: aaamodel.ResourceRef{Type: "system", ID: "llm-profiles"}},
		{action: "system.update", resource: aaamodel.ResourceRef{Type: "system", ID: "agent-profiles"}},
		{action: "system.update", resource: aaamodel.ResourceRef{Type: "system", ID: "mcp"}},
		{action: "system.update", resource: aaamodel.ResourceRef{Type: "system", ID: "config"}},
		{action: "system.update", resource: aaamodel.ResourceRef{Type: "system", ID: "notifications"}},
		{action: "git_webhook_source.create", resource: aaamodel.ResourceRef{Type: grantResourceGitWebhookSource, ID: "*"}},
		{action: "git_webhook_source.update", resource: aaamodel.ResourceRef{Type: grantResourceGitWebhookSource, ID: "*"}},
		{action: "team.update", resource: aaamodel.ResourceRef{Type: grantResourceTeam, ID: "*"}},
	}
}

func credentialIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	credentialID, err := uuid.Parse(strings.TrimSpace(r.PathValue("credentialID")))
	if err != nil {
		http.Error(w, "invalid credential id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return credentialID, true
}

func credentialActor(r *http.Request) string {
	if r == nil {
		return ""
	}
	return credentialActorFromContext(r.Context())
}

func credentialActorFromContext(ctx context.Context) string {
	if claims, ok := auth.ClaimsFromContext(ctx); ok && claims != nil {
		return strings.TrimSpace(claims.Sub)
	}
	return ""
}

func writeCredentialError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, credentials.ErrNotFound):
		http.Error(w, "credential not found", http.StatusNotFound)
	case errors.Is(err, credentials.ErrInvalidReference):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, credentials.ErrActiveVersion), errors.Is(err, credentials.ErrLastVersion):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		var duplicate interface{ SQLState() string }
		if errors.As(err, &duplicate) && duplicate.SQLState() == "23505" {
			http.Error(w, "credential reference already exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
