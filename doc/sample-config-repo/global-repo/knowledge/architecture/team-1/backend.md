---
name: backend
title: Team 1 Backend Architecture
kind: architecture
visibility: group
---

# Team 1 Backend Architecture

- Keep API changes backward compatible unless the pipeline goal explicitly requests a breaking change.
- Prefer small, reviewable changes that preserve existing deployment and runtime contracts.
- Check Docker and service configuration together when changing runtime behavior.
