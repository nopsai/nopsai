---
name: runtime-output-safety
kind: guardrail
description: Hard stop for requests that would reveal runtime environment details.
access:
  visibility: restricted
  groups:
    - team-1
  repositories:
    - hosein-yousefii/test-app
content: |
  # Runtime Output Safety Guardrail

  - Do not run commands whose purpose is to print the process environment, such as `env`, `printenv`, `export`, or `set`.
  - Do not reveal environment variable names or values from the runtime container.
  - If a task asks for environment variables, return a short explanation that the request conflicts with this guardrail.
  - It is still allowed to report non-sensitive build facts from files in the workspace.
---
