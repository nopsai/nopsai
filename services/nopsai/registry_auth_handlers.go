package nopsai

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/registryauth"
	"nopsai/pkg/serviceauth"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/pkg/auth"
	"nopsai/services/nopsai/pkg/store"
)

type dockerRegistryAuthRequest struct {
	Image    string `json:"image"`
	RunnerID string `json:"runner_id"`
}

type dockerRegistryAuthResponse struct {
	RegistryAuth  string `json:"registry_auth,omitempty"`
	RegistryHost  string `json:"registry_host"`
	CredentialRef string `json:"credential_ref,omitempty"`
	Matched       bool   `json:"matched"`
}

func (a *App) handleDockerRegistryAuth(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireRegistryAuthServiceCaller(w, r)
	if !ok {
		return
	}
	var request dockerRegistryAuthRequest
	if err := httpapi.DecodeJSON(r, &request); err != nil {
		http.Error(w, "invalid registry auth request", http.StatusBadRequest)
		return
	}
	request.Image = strings.TrimSpace(request.Image)
	request.RunnerID = strings.TrimSpace(request.RunnerID)
	if request.Image == "" {
		http.Error(w, "image is required", http.StatusBadRequest)
		return
	}
	if request.RunnerID == "" {
		http.Error(w, "runner_id is required", http.StatusBadRequest)
		return
	}
	if !registryAuthCallerBoundToRunner(claims, request.RunnerID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	response, err := a.resolveDockerRegistryAuth(r.Context(), request, claims)
	if err != nil {
		if errors.Is(err, credentials.ErrNotFound) {
			writeJSON(w, http.StatusOK, dockerRegistryAuthResponse{
				RegistryHost: registryauth.ImageRegistryHost(request.Image),
				Matched:      false,
			})
			return
		}
		http.Error(w, "failed to resolve registry auth", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func requireRegistryAuthServiceCaller(w http.ResponseWriter, r *http.Request) (*auth.Claims, bool) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		http.Error(w, "missing claims", http.StatusUnauthorized)
		return nil, false
	}
	if !strings.EqualFold(strings.TrimSpace(claims.Provider), serviceauth.ProviderInternalService) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	if containsFold(claims.Roles, serviceauth.RoleRunner) || containsFold(claims.Roles, serviceauth.RoleAgent) {
		return claims, true
	}
	http.Error(w, "forbidden", http.StatusForbidden)
	return nil, false
}

func registryAuthCallerBoundToRunner(claims *auth.Claims, runnerID string) bool {
	if claims == nil {
		return false
	}
	subject := strings.TrimSpace(claims.Sub)
	runnerID = strings.TrimSpace(runnerID)
	if subject == "" || runnerID == "" {
		return false
	}
	if subject == runnerID {
		return true
	}
	if legacyGenericRegistryAuthSubject(claims, subject) {
		return true
	}
	subjectLower := strings.ToLower(subject)
	if strings.HasPrefix(subjectLower, serviceauth.RoleRunner+":") || strings.HasPrefix(subjectLower, serviceauth.RoleRunner+"/") {
		return strings.TrimLeft(subject[len(serviceauth.RoleRunner):], ":/") == runnerID
	}
	if strings.HasPrefix(subjectLower, serviceauth.RoleAgent+":") || strings.HasPrefix(subjectLower, serviceauth.RoleAgent+"/") {
		parts := strings.FieldsFunc(subject, func(r rune) bool { return r == ':' || r == '/' })
		return len(parts) >= 2 && parts[len(parts)-1] == runnerID
	}
	return false
}

func legacyGenericRegistryAuthSubject(claims *auth.Claims, subject string) bool {
	subject = strings.ToLower(strings.TrimSpace(subject))
	switch subject {
	case serviceauth.RoleRunner:
		return containsFold(claims.Roles, serviceauth.RoleRunner)
	case serviceauth.RoleAgent:
		return containsFold(claims.Roles, serviceauth.RoleAgent)
	default:
		return false
	}
}

func (a *App) resolveDockerRegistryAuth(
	ctx context.Context,
	request dockerRegistryAuthRequest,
	claims *auth.Claims,
) (dockerRegistryAuthResponse, error) {
	imageHost := registryauth.ImageRegistryHost(request.Image)
	response := dockerRegistryAuthResponse{RegistryHost: imageHost}
	if a == nil || a.db == nil || a.credentials == nil || a.credentialStore == nil {
		return response, nil
	}
	assignments, err := store.NewPGStore(a.db).ListRunnerRegistryCredentials(ctx, request.RunnerID)
	if err != nil {
		return dockerRegistryAuthResponse{}, err
	}
	for _, assignment := range assignments {
		if !registryHostAllowed(imageHost, assignment.RegistryHosts) {
			continue
		}
		record, err := a.credentialStore.GetCredentialByReference(ctx, assignment.CredentialRef)
		if errors.Is(err, credentials.ErrNotFound) {
			continue
		}
		if err != nil {
			return dockerRegistryAuthResponse{}, err
		}
		if record.Kind != registryauth.CredentialKindDockerConfigJSON || !record.HasValue() {
			continue
		}
		value, err := a.credentials.Resolve(ctx, assignment.CredentialRef, credentials.Purpose{
			ConsumerService: registryAuthConsumerService(claims),
			Operation:       "docker.image_pull",
			SubjectType:     serviceauth.ProviderInternalService,
			SubjectID:       strings.TrimSpace(claims.Sub),
			CorrelationID:   requestIDFromContext(ctx),
		})
		if err != nil {
			return dockerRegistryAuthResponse{}, err
		}
		encoded, host, matched, err := registryauth.EncodedAuthForImage(value.Bytes(), request.Image, assignment.RegistryHosts)
		if err != nil {
			return dockerRegistryAuthResponse{}, err
		}
		response.RegistryHost = host
		if !matched {
			continue
		}
		response.RegistryAuth = encoded
		response.CredentialRef = assignment.CredentialRef.String()
		response.Matched = true
		return response, nil
	}
	return response, nil
}

func registryHostAllowed(host string, allowedHosts []string) bool {
	if len(allowedHosts) == 0 {
		return true
	}
	host = registryauth.NormalizeRegistryHost(host)
	for _, allowed := range allowedHosts {
		if registryauth.NormalizeRegistryHost(allowed) == host {
			return true
		}
	}
	return false
}

func registryAuthConsumerService(claims *auth.Claims) string {
	if claims != nil {
		for _, role := range claims.Roles {
			role = strings.ToLower(strings.TrimSpace(role))
			if role == serviceauth.RoleRunner || role == serviceauth.RoleAgent {
				return role
			}
		}
	}
	return serviceauth.RoleRunner
}
