import assert from "node:assert/strict";
import test from "node:test";
import {
  accessGrantMatchesUser,
  accessGrantEditKey,
  identityProviderPayloadFromForm,
  isExternallyManagedUser,
  isProtectedAccessRole,
  isUserRoleManagementLocked,
  normalizeAccessGrantRecord,
  normalizeBasicGrantInputs,
  normalizeIdentityProvidersState,
  userDisplayName,
  userProviderLabel,
  userSubjectLabel,
} from "./model.js";

test("normalizes and deduplicates enterprise basic access grants", () => {
  const grants = normalizeBasicGrantInputs([
    { role: "Owner", resourceType: "team", resourceID: "/platform/" },
    { role: "owner", resourceType: "team", resourceID: "platform" },
    { role: "admin", resourceType: "platform", resourceID: "platform" },
  ]);

  assert.equal(grants.length, 2);
  assert.equal(accessGrantEditKey(grants[0]), "owner::team::platform");
  assert.equal(accessGrantEditKey(grants[1]), "admin::platform::platform");
  assert.equal(isProtectedAccessRole("NopsAI-Admin"), true);
});

test("maps API access grant records into the UI contract", () => {
  assert.deepEqual(
    normalizeAccessGrantRecord({
      id: "grant-1",
      subject_type: "service_account",
      subject_id: "deploy-bot",
      role: "developer",
      resource_type: "team",
      resource_id: "platform",
      inherit: true,
      granted_by: "admin",
    }),
    {
      id: "grant-1",
      subjectType: "service_account",
      subjectID: "deploy-bot",
      subjectDisplay: undefined,
      role: "developer",
      resourceType: "team",
      resourceID: "platform",
      inherit: true,
      grantedBy: "admin",
      createdAt: undefined,
      managedByConfigRepo: false,
      managedByIdentityProvider: false,
      identityProviderID: undefined,
      externalTeamName: undefined,
      source: undefined,
    },
  );
});

test("formats externally managed OIDC users without exposing raw subject first", () => {
  const user = {
    id: "user-1",
    sub: "oidc:nopsai:7e9b8422-a701-4b4a-bf36-60b973fa98c6",
    email: "sso-admin@example.com",
    display_name: "sso-admin@example.com",
    provider: "oidc:nopsai",
    status: "active",
    external_managed: true,
    external_provider_id: "nopsai",
    external_provider_name: "Local Keycloak",
    external_subject: "7e9b8422-a701-4b4a-bf36-60b973fa98c6",
  };

  assert.equal(isExternallyManagedUser(user), true);
  assert.equal(isUserRoleManagementLocked(user), true);
  assert.equal(userDisplayName(user), "sso-admin@example.com");
  assert.equal(userProviderLabel(user), "Local Keycloak");
  assert.equal(userSubjectLabel(user), "7e9b8422-a701-4b4a-bf36-60b973fa98c6");
});

test("normalizes identity provider state and builds save payloads", () => {
  const state = normalizeIdentityProvidersState({
    settings: {
      local_enabled: false,
      oidc_enabled: true,
      auto_create_users: true,
      default_role: "viewer",
      allow_email_linking: false,
    },
    domain_mappings: {
      "company.com": "corporate",
    },
    providers: [
      {
        id: "corporate",
        type: "oidc",
        display_name: "Company SSO",
        issuer: "https://idp.company.com",
        client_id: "client-id",
        scopes: ["openid", "email"],
        allowed_email_domains: ["company.com"],
        role_mapping: { "nopsai-admins": "admin" },
        team_mapping: { "nopsai-admins": "sso-admins" },
        basic_role_mapping: {
          "team-1-owner": { role: "owner", resource: "team:team-1" },
        },
        client_credential_ref: "credential://system/oidc/corporate/client-secret",
        enabled: true,
        has_client_credential: true,
      },
    ],
  });

  assert.equal(state.settings.local_enabled, false);
  assert.equal(state.settings.default_role, "viewer");
  assert.equal(state.providers[0].display_name, "Company SSO");
  assert.deepEqual(state.providers[0].team_mapping, { "nopsai-admins": "sso-admins" });
  assert.deepEqual(state.providers[0].basic_role_mapping, {
    "team-1-owner": {
      role: "owner",
      resource: "team:team-1",
      resource_type: undefined,
      resource_id: undefined,
    },
  });
  assert.deepEqual(state.domain_mappings, { "company.com": "corporate" });

  assert.deepEqual(
    identityProviderPayloadFromForm({
      id: "corporate",
      type: "oidc",
      display_name: "Company SSO",
      issuer: "https://idp.company.com",
      authorization_endpoint: "",
      token_endpoint: "",
      jwks_uri: "",
      userinfo_endpoint: "",
      client_id: "client-id",
      client_credential_ref: "credential://system/oidc/corporate/client-secret",
      scopes: "openid, email, profile",
      allowed_email_domains: "company.com",
      team_claim: "teams",
      role_mapping: "nopsai-admins: admin",
      team_mapping: "nopsai-admins: sso-admins",
      basic_role_mapping: "team-1-owner: owner team:team-1",
      auto_create_users: "inherit",
      default_role: "viewer",
      allow_email_linking: "false",
      enabled: true,
    }),
    {
      id: "corporate",
      type: "oidc",
      display_name: "Company SSO",
      issuer: "https://idp.company.com",
      authorization_endpoint: "",
      token_endpoint: "",
      jwks_uri: "",
      userinfo_endpoint: "",
      client_id: "client-id",
      client_credential_ref: "credential://system/oidc/corporate/client-secret",
      scopes: ["openid", "email", "profile"],
      allowed_email_domains: ["company.com"],
      team_claim: "teams",
      role_mapping: { "nopsai-admins": "admin" },
      team_mapping: { "nopsai-admins": "sso-admins" },
      basic_role_mapping: {
        "team-1-owner": { role: "owner", resource: "team:team-1" },
      },
      auto_create_users: undefined,
      default_role: "viewer",
      allow_email_linking: false,
      enabled: true,
    },
  );
});

test("normalizes missing identity provider default role as empty", () => {
  const state = normalizeIdentityProvidersState({
    settings: {
      local_enabled: true,
      oidc_enabled: true,
      auto_create_users: true,
    },
  });

  assert.equal(state.settings.default_role, "");
});

test("matches external users through mapped NopsAI auth teams", () => {
  const user = {
    id: "user-1",
    sub: "oidc:nopsai:subject",
    email: "sso-owner@example.com",
    provider: "oidc:nopsai",
    status: "active",
    external_managed: true,
    external_auth_teams: [{ id: "team-1", name: "sso-owners" }],
  };

  assert.equal(
    accessGrantMatchesUser(
      {
        id: "grant-1",
        subjectType: "auth_team",
        subjectID: "team-1",
        role: "owner",
        resourceType: "team",
        resourceID: "team-1",
        inherit: true,
      },
      user,
    ),
    true,
  );
  assert.equal(
    accessGrantMatchesUser(
      {
        id: "grant-2",
        subjectType: "auth_team",
        subjectID: "sso-viewers",
        role: "viewer",
        resourceType: "team",
        resourceID: "team-1",
        inherit: true,
      },
      user,
    ),
    false,
  );
});
