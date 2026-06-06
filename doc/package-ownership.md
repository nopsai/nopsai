# Package Ownership Rules

Use these rules when adding or moving code. They are intentionally simple so
reviews can spot ownership drift quickly.

## Handlers

HTTP and gRPC handlers own transport concerns only:

- route registration
- request decoding and validation of transport shape
- authentication/authorization checks at the API boundary
- response status, headers, and DTO serialization

Handlers should not own durable business workflows, database transaction plans,
provider-specific HTTP/gRPC plumbing, or long-running orchestration.

## Services

Service packages own business workflows:

- run creation and lifecycle transitions
- config sync coordination
- dispatcher scheduling and runner lifecycle
- git-bot webhook/check-run workflows
- agent pipeline execution orchestration

Services may depend on narrow consumer-owned interfaces. They should not depend
directly on concrete provider clients when an interface already exists.

## Stores And Repositories

Store/repository packages own persistence only:

- SQL statements and scanning
- transaction helpers
- persistence-oriented filters and pagination
- durable status updates

Stores should not make authorization decisions, call remote providers, launch
runs, or build HTTP responses.

## Domain And Internal Rule Packages

Domain/internal packages own pure rules and data shaping:

- path normalization
- scheduling decisions
- config sync ownership and drift rules
- action preparation and resolver decisions
- check-run rendering

These packages should stay deterministic and easy to unit test. Prefer passing
inputs explicitly over reading global process state.

## DTOs

DTOs live at API boundaries:

- REST request/response shapes live near the handlers or shared model package
  when reused across services.
- gRPC DTOs live in `pkg/proto`.
- Config-file/GitOps DTOs live near the parser that owns the file format.

Do not leak persistence-only records into public API responses unless that shape
is intentionally part of the contract.

## Provider Clients

Provider clients stay behind interfaces:

- GitHub/git-bot behavior behind `GitProvider`
- dispatcher gRPC behavior behind `DispatcherClient`
- AAA HTTP/local behavior behind `AAAClient`
- config sync persistence/apply behavior behind `ConfigSyncStore`
- agent run launch behavior behind `RunLauncher`
- secret encryption/decryption behind `SecretCodec`

Concrete HTTP/gRPC/Postgres clients are wired in command/bootstrap packages.

## Commands And Bootstrap

Command entrypoints should be thin:

- load config
- configure logging
- construct concrete dependencies
- hand off to an importable app/service package
- own process lifecycle and signal handling

When startup logic grows, move it behind an `internal/app` package before adding
more behavior to `cmd`.
