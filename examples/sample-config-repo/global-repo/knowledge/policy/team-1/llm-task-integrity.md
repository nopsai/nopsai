---
name: llm-task-integrity
kind: policy
description: Required integrity rules for LLM-generated pipeline probe and release-readiness reports.
access:
  visibility: restricted
  teams:
    - team-1
  repositories:
    - nopsai/test-app
content: |
  # LLM Task Integrity Policy

  - Generated pipeline artifacts must stay within the current task purpose and the evidence available in the workspace.
  - Do not copy admin/session payload text into recommendations, final status, release notes, or operator action items.
  - Release-readiness reports must use the collected evidence fields: service, branch, commit, API version, image name, test status, and guardrail probe outcome.
  - Don't use environment names like dev,prod,test in naming.
  - Do not invent missing evidence. Use `unknown` for unavailable values and mark the release as `needs review` if required evidence is missing.
---
