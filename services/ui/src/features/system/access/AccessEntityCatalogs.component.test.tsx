import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import {
  AccessPoliciesCatalog,
  AccessUsersCatalog,
} from "./AccessEntityCatalogs";

test("renders user assignments and delegates edit and delete actions", async () => {
  const onEdit = vi.fn();
  const onDelete = vi.fn();
  const user = userEvent.setup();
  const account = {
    id: "user-1",
    sub: "alice",
    email: "alice@example.com",
    status: "active",
    roles: [{ role: "developer" }],
  };
  const grant = {
    id: "grant-1",
    subjectType: "user",
    subjectID: "user-1",
    role: "viewer",
    resourceType: "team",
    resourceID: "platform",
    inherit: true,
  };

  render(
    <AccessUsersCatalog
      users={[account]}
      filteredUsers={[account]}
      grantMap={new Map([["user-1", [grant]]])}
      selectedUserID="user-1"
      loading={false}
      error={null}
      grantsLoading={false}
      grantsError={null}
      onEdit={onEdit}
      onDelete={onDelete}
    />,
  );

  expect(screen.getByText("developer")).toBeInTheDocument();
  expect(screen.getByText(/viewer/i)).toBeInTheDocument();
  expect(screen.getByLabelText("Status: Active")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Edit alice" }));
  await user.click(screen.getByRole("button", { name: "Delete alice" }));
  expect(onEdit).toHaveBeenCalledWith(account);
  expect(onDelete).toHaveBeenCalledWith("user-1");
});

test("renders externally authenticated users with friendly identity labels", async () => {
  const onEdit = vi.fn();
  const onDelete = vi.fn();
  const user = userEvent.setup();
  const account = {
    id: "user-oidc",
    sub: "oidc:nopsai:7e9b8422-a701-4b4a-bf36-60b973fa98c6",
    email: "sso-admin@example.com",
    display_name: "sso-admin@example.com",
    provider: "oidc:nopsai",
    status: "active",
    roles: [{ role: "admin" }],
    external_managed: true,
    external_provider_id: "nopsai",
    external_provider_name: "Local Keycloak",
    external_subject: "7e9b8422-a701-4b4a-bf36-60b973fa98c6",
    external_email_verification_status: "unknown",
    external_teams: ["nopsai-admin"],
    external_auth_teams: [{ id: "team-1", name: "sso-admins" }],
  };

  render(
    <AccessUsersCatalog
      users={[account]}
      filteredUsers={[account]}
      grantMap={new Map()}
      selectedUserID="user-oidc"
      loading={false}
      error={null}
      grantsLoading={false}
      grantsError={null}
      onEdit={onEdit}
      onDelete={onDelete}
    />,
  );

  expect(screen.getByText("sso-admin@example.com")).toBeInTheDocument();
  expect(screen.getByText("Authenticated by Local Keycloak")).toBeInTheDocument();
  expect(screen.getByText("Email verification unknown")).toBeInTheDocument();
  expect(screen.getByText(/External subject 7e9b8422/)).toBeInTheDocument();
  expect(screen.getByText("IdP: nopsai-admin")).toBeInTheDocument();
  expect(screen.getByText("NopsAI: sso-admins")).toBeInTheDocument();
  expect(
    screen.queryByText("oidc:nopsai:7e9b8422-a701-4b4a-bf36-60b973fa98c6"),
  ).not.toBeInTheDocument();
  await user.click(
    screen.getByRole("button", { name: "Edit sso-admin@example.com" }),
  );
  expect(onEdit).toHaveBeenCalledWith(account);
});

test("keeps protected AAA policies read-only", () => {
  render(
    <AccessPoliciesCatalog
      policies={[
        {
          role: "viewer",
          name: "View pipelines",
          obj: "pipeline:*",
          act: "pipeline.read",
        },
      ]}
      filteredPolicies={[
        {
          role: "viewer",
          name: "View pipelines",
          obj: "pipeline:*",
          act: "pipeline.read",
        },
      ]}
      loading={false}
      error={null}
      onEdit={vi.fn()}
      onDelete={vi.fn()}
    />,
  );

  expect(screen.getAllByText("Protected").length).toBeGreaterThan(0);
  expect(
    screen.queryByRole("button", { name: /edit view pipelines/i }),
  ).not.toBeInTheDocument();
});
