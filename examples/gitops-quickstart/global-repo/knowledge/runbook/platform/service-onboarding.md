---
name: service-onboarding
kind: runbook
description: Service onboarding runbook mirrored from the engineering wiki.
access:
  visibility: team
# The page body is mirrored from the connected page at run time, so this file
# declares which page is attached and how it syncs, not the page content.
source:
  type: external_page
  connection: engineering-wiki
  provider: notion
  page_id: 8a7f0c1149e64a2f9c2b1c9a0e5d4f31
  page_url: https://www.notion.so/acme/Service-Onboarding-8a7f0c1149e64a2f9c2b1c9a0e5d4f31
  page_title: Service Onboarding
  sync:
    mode: periodic
    interval_minutes: 120
    failure_mode: use_cached
---
