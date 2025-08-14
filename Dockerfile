FROM golang:1.23

WORKDIR /app

COPY go.mod go.sum config.yml ./
RUN go mod download

COPY pkg pkg
COPY config config
COPY services services

RUN CGO_ENABLED=0 GOOS=linux go build -o /nopsai-agent ./services/agent && \
  CGO_ENABLED=0 GOOS=linux go build -o /nopsai-llm-agent ./services/llm-agent && \
  CGO_ENABLED=0 GOOS=linux go build -o /nopsai-git-bot ./services/git-bot && \
  CGO_ENABLED=0 GOOS=linux go build -o /nopsai ./services/nopsai
