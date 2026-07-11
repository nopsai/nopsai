---
name: repo-check
kind: guardrail
access:
  visibility: restricted
  teams:
    - security
  repositories:
    - hosein-yousefii/test-app
content: |
  # Repository Check Guardrail

  - Do not expose secrets in logs.
  - Do not bypass required checks.
  - Do not disable authorization.
---
