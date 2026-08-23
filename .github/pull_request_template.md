## What this changes

<!-- One or two sentences. What behaviour is different after this merges? -->

## Why

<!-- The problem being solved, not a restatement of the diff. -->

## Verification

<!-- What you ran, and what it showed. "Tests pass" on its own is not enough. -->

- [ ] `scripts/test-backend.sh`
- [ ] `npm test` in `services/ui`
- [ ] `scripts/enterprise-gates.sh` when touching release, container, or gate material

## Distribution checklist

Tick only what applies. NopsAI images are publicly pullable, so anything added
here reaches everyone who pulls.

- [ ] No credential, token, private key, or customer data is added, including in
      test fixtures and example files
- [ ] New third-party dependencies pass `scripts/license-check.sh`
- [ ] User-facing surfaces stay readable in dark mode
- [ ] New CLI commands are reachable from the interactive console
- [ ] New configuration is Git-owned rather than set only at runtime
