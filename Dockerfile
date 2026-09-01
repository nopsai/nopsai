FROM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG API_VERSION
ARG RUNNER_PROTOCOL_VERSION
ARG CLI_COMPATIBILITY
ARG RUNNER_COMPATIBILITY
ARG PLATFORM_COMPATIBILITY
ARG CAPABILITIES

WORKDIR /src
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY version.go version.txt ./
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
RUN BUILD_LDFLAGS="-s -w -X nopsai/pkg/buildinfo.Version=${VERSION} -X nopsai/pkg/buildinfo.Commit=${COMMIT} -X nopsai/pkg/buildinfo.BuildDate=${BUILD_DATE} -X nopsai/pkg/buildinfo.APIVersion=${API_VERSION} -X nopsai/pkg/buildinfo.RunnerProtocolVersion=${RUNNER_PROTOCOL_VERSION} -X nopsai/pkg/buildinfo.CLICompatibility=${CLI_COMPATIBILITY} -X nopsai/pkg/buildinfo.RunnerCompatibility=${RUNNER_COMPATIBILITY} -X nopsai/pkg/buildinfo.PlatformCompatibility=${PLATFORM_COMPATIBILITY} -X nopsai/pkg/buildinfo.Capabilities=${CAPABILITIES}" && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-agent ./services/agent/cmd/agent && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-git-bot ./services/git-bot/cmd/git-bot && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai ./cmd/nopsai-cli && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-api ./services/nopsai/cmd/nopsai && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-aaa ./services/aaa && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-dispatcher ./services/dispatcher/cmd/dispatcher && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-docker-runner ./services/docker-runner/cmd/docker-runner && \
  go build -ldflags="${BUILD_LDFLAGS}" -o /out/nopsai-k8s-runner ./services/k8s-runner/cmd/k8s-runner && \
  go build -trimpath -ldflags="${BUILD_LDFLAGS}" -o /out/docker-socket-proxy ./services/docker-socket-proxy/cmd/socket-proxy

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG SOURCE_URL=https://github.com/nopsai/nopsai

LABEL org.opencontainers.image.version="${VERSION}" \
  org.opencontainers.image.revision="${COMMIT}" \
  org.opencontainers.image.created="${BUILD_DATE}" \
  org.opencontainers.image.source="${SOURCE_URL}" \
  org.opencontainers.image.licenses="PolyForm-Noncommercial-1.0.0" \
  org.opencontainers.image.vendor="NopsAI"

RUN apk add --no-cache ca-certificates && \
  addgroup -S nopsai && adduser -S nopsai -G nopsai

WORKDIR /app
COPY --from=builder /out/nopsai-agent /nopsai-agent
COPY --from=builder /out/nopsai-git-bot /nopsai-git-bot
COPY --from=builder /out/nopsai /nopsai
COPY --from=builder /out/nopsai-api /nopsai-api
COPY --from=builder /out/nopsai-aaa /nopsai-aaa
COPY --from=builder /out/nopsai-dispatcher /nopsai-dispatcher
COPY --from=builder /out/nopsai-docker-runner /nopsai-docker-runner
COPY --from=builder /out/nopsai-k8s-runner /nopsai-k8s-runner
COPY --from=builder /out/docker-socket-proxy /docker-socket-proxy
COPY --from=builder /src/config.yml /app/config.yml
COPY LICENSE THIRD_PARTY_NOTICES.md /usr/share/licenses/nopsai/

USER nopsai
ENTRYPOINT ["/bin/true"]
