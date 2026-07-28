# NopsAI Examples

This directory contains runnable or copyable examples. Keep examples here so
`doc/` can stay focused on explanatory guides and reference material.

| Path | Purpose |
| --- | --- |
| `sample-config-repo/` | Full GitOps sample with global and team config repositories, pipelines, scopes, triggers, dashboards, access grants, identity-provider settings, MCP, mail, runner, and credential-reference examples. |
| `sample-pipeline/` | Standalone pipeline and trigger YAML examples for testing pipeline schema features without adopting the full GitOps sample repository. |
| `sso/` | Local Keycloak and multi-provider IdP fixtures for SSO integration testing. |

## Ownership Boundary

Examples are not production runtime configuration. Copy the relevant files into
your own config repository or deployment environment, review values, replace
placeholder credentials with credential references, and then sync through the
normal NopsAI GitOps or API flows.

Documentation and the in-app wiki should link to these examples as source
evidence when describing runnable sample layouts.
