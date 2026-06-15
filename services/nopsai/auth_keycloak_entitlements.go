package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"nopsai/services/nopsai/internal/credentials"

	"github.com/rs/zerolog/log"
)

type keycloakTokenResponse struct {
	AccessToken string `json:"access_token"`
}

type keycloakClientRepresentation struct {
	ID       string `json:"id"`
	ClientID string `json:"clientId"`
}

type keycloakGroupRepresentation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type keycloakRoleRepresentation struct {
	Name string `json:"name"`
}

func (a *App) enrichOIDCIdentityEntitlements(ctx context.Context, provider oidcProviderRecord, identity oidcVerifiedIdentity) (oidcVerifiedIdentity, error) {
	sync := normalizeOIDCEntitlementSync(provider.EntitlementSync)
	if sync.Mode != "keycloak_group_roles" {
		return identity, nil
	}
	if sync.AdminBaseURL == "" || sync.Realm == "" {
		return identity, fmt.Errorf("keycloak entitlement sync requires admin_base_url and realm")
	}

	adminToken, err := a.keycloakAdminToken(ctx, sync)
	if err != nil {
		return identity, err
	}
	clientID := firstNonEmptyString(sync.ClientID, provider.ClientID)
	if clientID == "" {
		return identity, fmt.Errorf("keycloak entitlement sync requires client_id")
	}
	clientUUID, err := a.keycloakClientUUID(ctx, sync, adminToken, clientID)
	if err != nil {
		return identity, err
	}

	directRoles, err := a.keycloakUserClientRoles(ctx, sync, adminToken, identity.Subject, clientUUID)
	if err != nil {
		return identity, err
	}
	for _, role := range directRoles {
		if normalized, ok := normalizeExternalAccessRoleName(role.Name); ok {
			identity.AccessRoles = append(identity.AccessRoles, normalized)
		}
	}

	groups, err := a.keycloakUserGroups(ctx, sync, adminToken, identity.Subject)
	if err != nil {
		return identity, err
	}
	for _, group := range groups {
		if strings.TrimSpace(group.ID) == "" {
			identity.Groups = appendOIDCGroup(identity.Groups, firstNonEmptyString(group.Path, group.Name))
			log.Warn().Str("provider", provider.ID).Str("group", group.Name).Str("path", group.Path).Msg("Skipping Keycloak group entitlement with empty group ID.")
			continue
		}
		if strings.TrimSpace(group.Path) == "" {
			detail, err := a.keycloakGroup(ctx, sync, adminToken, group.ID)
			if err != nil {
				return identity, err
			}
			group = mergeKeycloakGroupRepresentation(group, detail)
		}
		identity.Groups = appendOIDCGroup(identity.Groups, firstNonEmptyString(group.Path, group.Name))
		roles, err := a.keycloakGroupClientRoles(ctx, sync, adminToken, group.ID, clientUUID)
		if err != nil {
			return identity, err
		}
		target := keycloakGroupTarget(sync, group)
		if target == "" {
			log.Warn().Str("provider", provider.ID).Str("group", group.Name).Str("path", group.Path).Msg("Skipping Keycloak group entitlement with empty NopsAI target.")
			continue
		}
		for _, role := range roles {
			basicRole, ok := normalizeExternalBasicRoleName(role.Name)
			if !ok {
				continue
			}
			identity.BasicRoles = append(identity.BasicRoles, oidcDesiredBasicRoleGrant{
				ExternalGroup: firstNonEmptyString(group.Path, group.Name),
				Role:          basicRole,
				ResourceType:  sync.TargetResourceType,
				ResourceID:    target,
				Inherit:       true,
			})
		}
	}
	return identity, nil
}

func (a *App) keycloakAdminToken(ctx context.Context, sync oidcEntitlementSyncConfig) (string, error) {
	adminPassword, err := a.resolveCredentialText(ctx, sync.AdminPasswordCredentialRef, credentials.Purpose{
		ConsumerService: "nopsai",
		Operation:       "oidc.keycloak_admin_password",
		SubjectType:     "identity_provider",
		SubjectID:       sync.Realm,
	})
	if err != nil {
		return "", fmt.Errorf("resolve Keycloak admin password credential: %w", err)
	}
	adminClientSecret, err := a.resolveCredentialText(ctx, sync.AdminClientCredentialRef, credentials.Purpose{
		ConsumerService: "nopsai",
		Operation:       "oidc.keycloak_admin_client",
		SubjectType:     "identity_provider",
		SubjectID:       sync.Realm,
	})
	if err != nil {
		return "", fmt.Errorf("resolve Keycloak admin client credential: %w", err)
	}
	form := url.Values{}
	form.Set("client_id", sync.AdminClientID)
	switch {
	case sync.AdminUsername != "" || sync.AdminPasswordCredentialRef != "":
		if sync.AdminUsername == "" || adminPassword == "" {
			return "", fmt.Errorf("keycloak password grant requires admin_username and admin_password_credential_ref")
		}
		form.Set("grant_type", "password")
		form.Set("username", sync.AdminUsername)
		form.Set("password", adminPassword)
		if adminClientSecret != "" {
			form.Set("client_secret", adminClientSecret)
		}
	case sync.AdminClientCredentialRef != "":
		form.Set("grant_type", "client_credentials")
		form.Set("client_secret", adminClientSecret)
	default:
		return "", fmt.Errorf("keycloak entitlement sync requires admin credentials")
	}
	tokenURL := sync.AdminBaseURL + "/realms/" + url.PathEscape(sync.AdminRealm) + "/protocol/openid-connect/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := a.oidcHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("keycloak admin token request failed with status %d", resp.StatusCode)
	}
	var token keycloakTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return "", err
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return "", fmt.Errorf("keycloak admin token response did not include access_token")
	}
	return token.AccessToken, nil
}

func (a *App) keycloakClientUUID(ctx context.Context, sync oidcEntitlementSyncConfig, token, clientID string) (string, error) {
	var clients []keycloakClientRepresentation
	path := "/admin/realms/" + url.PathEscape(sync.Realm) + "/clients?clientId=" + url.QueryEscape(clientID)
	if err := a.keycloakAdminGET(ctx, sync, token, path, &clients); err != nil {
		return "", err
	}
	for _, client := range clients {
		if client.ClientID == clientID && client.ID != "" {
			return client.ID, nil
		}
	}
	return "", fmt.Errorf("keycloak client %q not found", clientID)
}

func (a *App) keycloakUserClientRoles(ctx context.Context, sync oidcEntitlementSyncConfig, token, userID, clientUUID string) ([]keycloakRoleRepresentation, error) {
	var roles []keycloakRoleRepresentation
	path := "/admin/realms/" + url.PathEscape(sync.Realm) + "/users/" + url.PathEscape(userID) + "/role-mappings/clients/" + url.PathEscape(clientUUID)
	if err := a.keycloakAdminGET(ctx, sync, token, path, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

func (a *App) keycloakUserGroups(ctx context.Context, sync oidcEntitlementSyncConfig, token, userID string) ([]keycloakGroupRepresentation, error) {
	var groups []keycloakGroupRepresentation
	path := "/admin/realms/" + url.PathEscape(sync.Realm) + "/users/" + url.PathEscape(userID) + "/groups?briefRepresentation=false"
	if err := a.keycloakAdminGET(ctx, sync, token, path, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func (a *App) keycloakGroup(ctx context.Context, sync oidcEntitlementSyncConfig, token, groupID string) (keycloakGroupRepresentation, error) {
	var group keycloakGroupRepresentation
	path := "/admin/realms/" + url.PathEscape(sync.Realm) + "/groups/" + url.PathEscape(groupID)
	if err := a.keycloakAdminGET(ctx, sync, token, path, &group); err != nil {
		return keycloakGroupRepresentation{}, err
	}
	return group, nil
}

func (a *App) keycloakGroupClientRoles(ctx context.Context, sync oidcEntitlementSyncConfig, token, groupID, clientUUID string) ([]keycloakRoleRepresentation, error) {
	var roles []keycloakRoleRepresentation
	path := "/admin/realms/" + url.PathEscape(sync.Realm) + "/groups/" + url.PathEscape(groupID) + "/role-mappings/clients/" + url.PathEscape(clientUUID) + "/composite"
	if err := a.keycloakAdminGET(ctx, sync, token, path, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

func mergeKeycloakGroupRepresentation(base, detail keycloakGroupRepresentation) keycloakGroupRepresentation {
	if strings.TrimSpace(base.ID) == "" {
		base.ID = detail.ID
	}
	if strings.TrimSpace(base.Name) == "" {
		base.Name = detail.Name
	}
	if strings.TrimSpace(base.Path) == "" {
		base.Path = detail.Path
	}
	return base
}

func (a *App) keycloakAdminGET(ctx context.Context, sync oidcEntitlementSyncConfig, token, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sync.AdminBaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := a.oidcHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("keycloak admin request failed with status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func keycloakGroupTarget(sync oidcEntitlementSyncConfig, group keycloakGroupRepresentation) string {
	target := strings.Trim(strings.TrimSpace(firstNonEmptyString(group.Path, group.Name)), "/")
	prefix := strings.Trim(strings.TrimSpace(sync.GroupPathPrefix), "/")
	if prefix != "" {
		if target == prefix {
			return ""
		}
		target = strings.TrimPrefix(target, prefix+"/")
	}
	return strings.Trim(target, "/")
}

func appendOIDCGroup(groups []string, group string) []string {
	group = strings.TrimSpace(group)
	if group == "" {
		return groups
	}
	for _, existing := range groups {
		if strings.TrimSpace(existing) == group {
			return groups
		}
	}
	return append(groups, group)
}

func normalizeExternalAccessRoleName(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case strings.ToLower(defaultAdminRole):
		return defaultAdminRole, true
	case productRoleAdmin:
		return productRoleAdmin, true
	case productRoleOwner:
		return productRoleOwner, true
	case productRoleDeveloper:
		return productRoleDeveloper, true
	case productRoleViewer:
		return productRoleViewer, true
	default:
		return "", false
	}
}

func normalizeExternalBasicRoleName(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case productRoleOwner:
		return productRoleOwner, true
	case productRoleDeveloper:
		return productRoleDeveloper, true
	case productRoleViewer:
		return productRoleViewer, true
	default:
		return "", false
	}
}
