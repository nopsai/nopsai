FROM golang:1.26.5-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG RELEASE_MANIFEST_DIGEST=
ARG API_VERSION=v1
ARG RUNNER_PROTOCOL_VERSION=1
ARG CLI_COMPATIBILITY=>=2.0.0,<3.0.0
ARG RUNNER_COMPATIBILITY=>=2.0.0,<3.0.0
ARG PLATFORM_COMPATIBILITY=>=2.0.0,<3.0.0
ARG CAPABILITIES=api.v1,cli.api-catalog.v1,config-sync.v1,mcp.v1,monitoring.v1,platform.docker-compose,platform.helm,runner.docker,runner.kubernetes

WORKDIR /src
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY pkg pkg
COPY config config
COPY db db
COPY services services
COPY internal internal
COPY cmd cmd
COPY config.yml .

ENV CGO_ENABLED=0
ENV GOOS=linux
RUN mkdir -p /out
RUN BUILD_LDFLAGS="-s -w -X nopsai/pkg/buildinfo.Version=${VERSION} -X nopsai/pkg/buildinfo.Commit=${COMMIT} -X nopsai/pkg/buildinfo.BuildDate=${BUILD_DATE} -X nopsai/pkg/buildinfo.ReleaseManifestDigest=${RELEASE_MANIFEST_DIGEST} -X nopsai/pkg/buildinfo.APIVersion=${API_VERSION} -X nopsai/pkg/buildinfo.RunnerProtocolVersion=${RUNNER_PROTOCOL_VERSION} -X nopsai/pkg/buildinfo.CLICompatibility=${CLI_COMPATIBILITY} -X nopsai/pkg/buildinfo.RunnerCompatibility=${RUNNER_COMPATIBILITY} -X nopsai/pkg/buildinfo.PlatformCompatibility=${PLATFORM_COMPATIBILITY} -X nopsai/pkg/buildinfo.Capabilities=${CAPABILITIES}" && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-agent ./services/agent/cmd/agent && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-git-bot ./services/git-bot/cmd/git-bot && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai ./cmd/nopsai-cli && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-api ./services/nopsai/cmd/nopsai && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-aaa ./services/aaa && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-dispatcher ./services/dispatcher/cmd/dispatcher && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-runner ./services/docker-runner/cmd/docker-runner && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-k8s-runner ./services/k8s-runner/cmd/k8s-runner

FROM alpine:3.20

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.version="${VERSION}" \
  org.opencontainers.image.revision="${COMMIT}" \
  org.opencontainers.image.created="${BUILD_DATE}"

RUN apk add --no-cache ca-certificates && \
  addgroup -S nopsai && adduser -S nopsai -G nopsai

WORKDIR /app
COPY --from=builder /out/nopsai-agent /nopsai-agent
COPY --from=builder /out/nopsai-git-bot /nopsai-git-bot
COPY --from=builder /out/nopsai /nopsai
COPY --from=builder /out/nopsai-api /nopsai-api
COPY --from=builder /out/nopsai-aaa /nopsai-aaa
COPY --from=builder /out/nopsai-dispatcher /nopsai-dispatcher
COPY --from=builder /out/nopsai-runner /nopsai-runner
COPY --from=builder /out/nopsai-k8s-runner /nopsai-k8s-runner
COPY --from=builder /src/config.yml /app/config.yml

USER nopsai
ENTRYPOINT ["/bin/true"]
