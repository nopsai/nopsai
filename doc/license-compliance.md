# License Compliance

This is the engineering policy for keeping NopsAI commercially distributable.
It is not legal advice; legal/commercial counsel should sign off before a paid
enterprise release.

## Automated Gate

Run the dependency license gate from the repository root:

```bash
scripts/license-check.sh
```

To save the current inventories for release review:

```bash
LICENSE_REPORT_DIR=dist/license-report scripts/license-check.sh
```

The gate scans:

- Go modules from `go.mod`/`go.sum`.
- UI packages from `services/ui/node_modules`, or from `npm ci`/Docker fallback
  when dependencies are not installed locally.

The gate fails for unknown licenses, GPL/AGPL/LGPL/SSPL-style copyleft,
Commons Clause, Business Source License, PolyForm Noncommercial, and other
noncommercial terms. It allows common commercial-compatible licenses, while
still surfacing review/notice obligations.

Allowed license identifiers are based on SPDX identifiers where possible:
`Apache-2.0`, `BSD-2-Clause`, `BSD-3-Clause`, `BlueOak-1.0.0`, `CC-BY-4.0`,
`CC0-1.0`, `ISC`, `MIT`, `MIT-0`, `MPL-2.0`, and `Python-2.0`.

## Current Language Dependency Result

As of the July 14, 2026 audit, the automated gate reports no blocked licenses
in the Go or UI dependency graphs.

| Ecosystem | Current result |
| --- | --- |
| Go | Apache-2.0, MIT, BSD-style, ISC, and MPL-2.0. |
| UI | MIT, Apache-2.0, BSD-style, ISC, MIT-0, MPL-2.0, CC-BY-4.0, CC0-1.0, BlueOak-1.0.0, and Python-2.0. |

Review/notice licenses currently surfaced by the gate:

- `pgregory.net/rapid`: MPL-2.0, test/development Go dependency.
- `axe-core` and `@axe-core/playwright`: MPL-2.0, UI accessibility test tooling.
- `caniuse-lite`: CC-BY-4.0 browser support data used by the UI build toolchain.
- `lru-cache`: BlueOak-1.0.0 transitive UI dependency.
- `argparse`: Python-2.0 transitive UI dependency.

These are not current blockers, but release notes and third-party notices must
preserve the required license text and attribution.

## Product Policy

New runtime or build dependencies must pass `scripts/license-check.sh` before
merge. If a dependency introduces a review/notice license, the PR must explain
why it is needed and how notices are preserved.

Do not add dependencies under GPL, AGPL, LGPL, SSPL, Commons Clause, Business
Source License, PolyForm Noncommercial, source-available-only terms, trial-only
terms, or any license that restricts commercial sale without explicit approval.

Do not copy third-party source, examples, documentation, datasets, images,
icons, fonts, model prompts, or generated content into this repository unless
their license allows commercial redistribution and the attribution path is
documented. The UI should keep product assets under
`services/ui/public/brand`, and unused starter assets should not stay tracked.

Customer-provided data, Git repository content, secrets, run logs, and LLM
context remain customer-owned/runtime data. NopsAI must not bundle customer or
vendor data into release artifacts.

## Containers And Services

The language dependency gate does not prove container-image compliance. Release
review must separately inspect image SBOMs and layer notices for:

- `golang`, `alpine`, `node`, `nginx`, `postgres`, and Playwright test images.
- `gotenberg/gotenberg`, including Chromium, LibreOffice, fonts, and PDF tools
  inside the image.
- `quay.io/keycloak/keycloak` in local SSO fixtures.
- `ngrok/ngrok` in local tunnel fixtures.

Production release bundles should use versioned or digest-pinned images and
must not rely on `ngrok` development tunnels. Ngrok, GitHub, LLM providers, and
other hosted integrations also carry service/subscription terms that are
separate from open-source code licenses.

## MCP And Plugin Boundary

First-party hosted MCP tools are NopsAI product code and are covered by the
same dependency gate as the rest of the repository.

External MCP servers, MCP plugins, connectors, and tool providers configured in
`setting/system/mcp.yaml` are runtime integrations. They are not automatically
licensed for redistribution just because NopsAI can call them. Before NopsAI
ships a bundled MCP server, plugin, example tool, or connector package, record:

- SPDX license or commercial agreement.
- Source repository or vendor.
- Whether code, schemas, prompts, examples, or data are redistributed.
- Whether the integration is dev-only, customer-provided, or bundled.
- Required notices, attribution, and service terms.

GitOps-managed MCP profiles should reference approved external services only.
Do not put proprietary plugin source, vendor datasets, or copied documentation
into config repositories unless redistribution rights are confirmed.

## Release Checklist

Before an enterprise release:

1. Run `scripts/license-check.sh` and store the report with the release review.
2. Confirm no blocked or unknown language dependency licenses.
3. Review all review/notice licenses and update third-party notices.
4. Inspect container image SBOMs and image-layer notices.
5. Confirm runtime MCP/plugin/service integrations are not bundled without
   approved commercial terms.
6. Confirm generated docs, examples, data, and UI assets are first-party or
   licensed for commercial redistribution.

References:

- SPDX License List: https://spdx.org/licenses
- Apache License 2.0: https://www.apache.org/licenses/LICENSE-2.0
- Keycloak license: https://github.com/keycloak/keycloak/blob/main/LICENSE.txt
- Gotenberg license: https://github.com/gotenberg/gotenberg/blob/main/LICENSE
- Ngrok Terms of Service: https://ngrok.com/tos
