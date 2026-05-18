---
name: repo-check
title: Repository Check Guardrail
kind: guardrail
visibility: restricted
access:
  groups:
    - security
  repositories:
    - hosein-yousefii/test-app
---

# Repository Check Guardrail

- Do not expose secrets in logs.
- Do not bypass required checks.
- Do not disable authorization.
