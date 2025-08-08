FROM golang:1.23

WORKDIR /app
COPY . .
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o /nopsai-agent ./services/agent && \
  CGO_ENABLED=0 GOOS=linux go build -o /nopsai-llm-agent ./services/llm-agent && \
  CGO_ENABLED=0 GOOS=linux go build -o /nopsai ./services/nopsai
