FROM golang:1.23-alpine AS builder

WORKDIR /src
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY pkg pkg
COPY config config
COPY services services
COPY config.yml .

ENV CGO_ENABLED=0
ENV GOOS=linux
RUN mkdir -p /out
RUN go build -ldflags="-s -w" -o /out/nopsai-agent ./services/agent && \
  go build -ldflags="-s -w" -o /out/nopsai-git-bot ./services/git-bot && \
  go build -ldflags="-s -w" -o /out/nopsai ./services/nopsai && \
  go build -ldflags="-s -w" -o /out/nopsai-dispatcher ./services/dispatcher && \
  go build -ldflags="-s -w" -o /out/nopsai-runner ./services/runner

FROM alpine:3.20

RUN apk add --no-cache ca-certificates && \
  addgroup -S nopsai && adduser -S nopsai -G nopsai

WORKDIR /app
COPY --from=builder /out/nopsai-agent /nopsai-agent
COPY --from=builder /out/nopsai-git-bot /nopsai-git-bot
COPY --from=builder /out/nopsai /nopsai
COPY --from=builder /out/nopsai-dispatcher /nopsai-dispatcher
COPY --from=builder /out/nopsai-runner /nopsai-runner
COPY --from=builder /src/config.yml /app/config.yml

USER nopsai
ENTRYPOINT ["/bin/true"]
