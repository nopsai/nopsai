package nopsai

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"nopsai/pkg/httpapi"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/pkg/auth"
)

func (a *App) handleListCredentials(w http.ResponseWriter, r *http.Request) {
	records, err := a.credentialStore.ListCredentials(r.Context())
	if err != nil {
		http.Error(w, "failed to list credentials", http.StatusInternalServerError)
		return
	}
	response := credentialsResponse{Credentials: make([]credentialResponse, 0, len(records))}
	for _, record := range records {
		response.Credentials = append(response.Credentials, credentialResponseFromRecord(record))
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) handleCreateCredential(w http.ResponseWriter, r *http.Request) {
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
