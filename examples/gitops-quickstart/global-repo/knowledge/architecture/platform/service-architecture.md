---
name: service-architecture
kind: architecture
access:
  visibility: team
content: |
  # Service Architecture

  - `service-api` is the only public entry point and must stay backward compatible.
  - Configuration changes ship through this config repository, never by hand.
  - Every deployment runs build and test before it reaches the prod scope.
---
