#!/usr/bin/env python3
import json
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
CONFIG = ROOT / "mock-oauth2" / "config.json"
EXPECTED = {"alice", "bob", "carol", "dave", "erin", "frank"}

with CONFIG.open(encoding="utf-8") as handle:
    config = json.load(handle)

errors = []
for callback in config.get("tokenCallbacks", []):
    issuer = callback.get("issuerId", "<missing>")
    mappings = callback.get("requestMappings", [])
    if len(mappings) != len(EXPECTED):
        errors.append(f"{issuer}: expected {len(EXPECTED)} mappings, got {len(mappings)}")

    matched_users = set()
    for mapping in mappings:
        claims = mapping.get("claims", {})
        email = str(claims.get("email", "")).strip()
        subject = str(claims.get("sub", "")).strip()
        pattern = str(mapping.get("match", ""))

        if not email:
            errors.append(f"{issuer}: mapping {pattern!r} has no email")
        if claims.get("email_verified") is not True:
            errors.append(f"{issuer}: {email or pattern!r} is not email_verified=true")
        if not subject:
            errors.append(f"{issuer}: {email or pattern!r} has no sub")

        try:
            compiled = re.compile(pattern)
        except re.error as exc:
            errors.append(f"{issuer}: invalid mapping regex {pattern!r}: {exc}")
            continue

        username = email.split("@", 1)[0] if "@" in email else ""
        if username:
            matched_users.add(username)
            if compiled.fullmatch(username) is None:
                errors.append(f"{issuer}: {pattern!r} does not match username {username!r}")
            if compiled.fullmatch(email) is None:
                errors.append(f"{issuer}: {pattern!r} does not match email {email!r}")

    missing = EXPECTED - matched_users
    if missing:
        errors.append(f"{issuer}: missing expected users: {', '.join(sorted(missing))}")

if errors:
    print("Fixture validation failed:", file=sys.stderr)
    for error in errors:
        print(f"- {error}", file=sys.stderr)
    raise SystemExit(1)

print("Fixture validation passed: every login mapping matches username and email and includes sub, email, and email_verified=true.")
