---
name: release-evidence
kind: policy
description: Mandatory evidence rules for release readiness reports.
access:
  visibility: restricted
  teams:
    - team-1
  repositories:
    - hosein-yousefii/test-app
content: |
  # Release Evidence Policy

  - A release readiness report must include branch, commit, API version, image name, and test status.
  - Do not mark a release as ready when any required evidence is missing or unknown.
  - Do not invent missing values. Use `unknown` for unavailable evidence.
  - If test status is not `passed`, the report must say the release is not ready.
  - Write the report to a workspace file when the task asks for a release report.
---
