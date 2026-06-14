import assert from "node:assert/strict";
import { test } from "node:test";
import {
  isEmailLikeIdentifier,
  shouldUseLocalPasswordForIdentifier,
} from "./loginIdentifier.js";

test("detects email-like identifiers for SSO discovery", () => {
  assert.equal(isEmailLikeIdentifier("alice@example.com"), true);
  assert.equal(isEmailLikeIdentifier(" alice@example.com "), true);
  assert.equal(isEmailLikeIdentifier("alice"), false);
  assert.equal(isEmailLikeIdentifier("@example.com"), false);
  assert.equal(isEmailLikeIdentifier("alice@"), false);
});

test("routes username-like identifiers to local password login when SSO is available", () => {
  assert.equal(
    shouldUseLocalPasswordForIdentifier({
      identifier: "admin",
      localEnabled: true,
      ssoEnabled: true,
    }),
    true,
  );
  assert.equal(
    shouldUseLocalPasswordForIdentifier({
      identifier: "alice@example.com",
      localEnabled: true,
      ssoEnabled: true,
    }),
    false,
  );
  assert.equal(
    shouldUseLocalPasswordForIdentifier({
      identifier: "admin",
      localEnabled: false,
      ssoEnabled: true,
    }),
    false,
  );
});
