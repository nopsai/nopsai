package nopsai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const oidcEntitlementSyncInterval = 5 * time.Minute

type oidcLinkedIdentityRecord struct {
	UserID        uuid.UUID
	ProviderID    string
	Issuer        string
	Subject       string
	Email         string
	EmailVerified bool
}

func (a *App) runOIDCEntitlementSyncWorker(ctx context.Context) {
	a.reconcileOIDCEntitlementsAndLog(ctx)

	ticker := time.NewTicker(oidcEntitlementSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.reconcileOIDCEntitlementsAndLog(ctx)
		}
	}
}

func (a *App) reconcileOIDCEntitlementsAndLog(ctx context.Context) {
	if err := a.reconcileOIDCEntitlements(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to reconcile OIDC entitlements.")
	}
}

func (a *App) reconcileOIDCEntitlements(ctx context.Context) error {
	if a == nil || a.db == nil {
		return nil
	}

	settings, err := getOIDCSettings(ctx, a.db, a.cfg)
	if err != nil {
		return fmt.Errorf("load OIDC settings: %w", err)
	}
	if !settings.OIDCEnabled {
		return nil
	}

	providers, err := listOIDCProviders(ctx, a.db, true)
	if err != nil {
		return fmt.Errorf("list OIDC providers: %w", err)
	}
	providersByID := keycloakEntitlementProvidersByID(providers)
	if len(providersByID) == 0 {
		return nil
	}

	identities, err := a.listKeycloakEntitlementIdentities(ctx, providersByID)
	if err != nil {
		return err
	}
	if len(identities) == 0 {
		return nil
	}

	var synced, failed int
	for _, linked := range identities {
		provider, ok := providersByID[linked.ProviderID]
		if !ok {
			continue
		}
		identity := oidcVerifiedIdentity{
			ProviderID:    provider.ID,
			Issuer:        linked.Issuer,
			Subject:       linked.Subject,
			Email:         linked.Email,
			EmailVerified: linked.EmailVerified,
		}
		enriched, err := a.enrichOIDCIdentityEntitlements(ctx, provider, identity)
		if err != nil {
			failed++
			log.Warn().
				Err(err).
				Str("provider", provider.ID).
				Str("subject", linked.Subject).
				Str("email", linked.Email).
				Msg("Failed to refresh OIDC entitlements for linked user.")
			continue
		}
		if err := a.syncLinkedOIDCIdentityEntitlements(ctx, settings, provider, linked.UserID, enriched); err != nil {
			failed++
			log.Warn().
				Err(err).
				Str("provider", provider.ID).
				Str("subject", linked.Subject).
				Str("email", linked.Email).
				Msg("Failed to apply refreshed OIDC entitlements for linked user.")
			continue
		}
		synced++
	}

	if synced > 0 || failed > 0 {
		log.Info().
			Int("synced", synced).
			Int("failed", failed).
			Int("linked_identities", len(identities)).
			Msg("Reconciled OIDC entitlements.")
	}
	return nil
}

func keycloakEntitlementProvidersByID(providers []oidcProviderRecord) map[string]oidcProviderRecord {
	out := map[string]oidcProviderRecord{}
	for _, provider := range providers {
		sync := normalizeOIDCEntitlementSync(provider.EntitlementSync)
		if sync.Mode != "keycloak_group_roles" {
			continue
		}
		provider.ID = normalizeOIDCProviderID(provider.ID)
		provider.EntitlementSync = sync
		if provider.ID == "" {
			continue
		}
		out[provider.ID] = provider
	}
	return out
}

func (a *App) listKeycloakEntitlementIdentities(ctx context.Context, providers map[string]oidcProviderRecord) ([]oidcLinkedIdentityRecord, error) {
	providerIDs := make([]string, 0, len(providers))
	for providerID := range providers {
		providerIDs = append(providerIDs, providerID)
	}
	if len(providerIDs) == 0 {
		return nil, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT user_id, provider_id, issuer, subject, email, email_verified
		FROM auth_external_identities
		WHERE provider_id = ANY($1::text[])
		ORDER BY provider_id, user_id
	`, providerIDs)
	if err != nil {
		return nil, fmt.Errorf("list linked OIDC identities: %w", err)
	}
	defer rows.Close()

	var identities []oidcLinkedIdentityRecord
	for rows.Next() {
		var identity oidcLinkedIdentityRecord
		if err := rows.Scan(
			&identity.UserID,
			&identity.ProviderID,
			&identity.Issuer,
			&identity.Subject,
			&identity.Email,
			&identity.EmailVerified,
		); err != nil {
			return nil, fmt.Errorf("scan linked OIDC identity: %w", err)
		}
		identity.ProviderID = normalizeOIDCProviderID(identity.ProviderID)
		identity.Subject = strings.TrimSpace(identity.Subject)
		if identity.ProviderID == "" || identity.Subject == "" {
			continue
		}
		identities = append(identities, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list linked OIDC identities: %w", err)
	}
	return identities, nil
}

func (a *App) syncLinkedOIDCIdentityEntitlements(ctx context.Context, settings oidcSettings, provider oidcProviderRecord, userID uuid.UUID, identity oidcVerifiedIdentity) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := syncOIDCRolesAndGroups(ctx, tx, userID, provider, settings, identity); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
