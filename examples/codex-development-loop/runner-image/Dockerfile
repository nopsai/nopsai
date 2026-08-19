# Container image for the Codex development loop.
#
# The loop's pipelines run every step in this image, so it has to carry three
# things: Git, the Codex CLI, and the target repository's own toolchain. The
# toolchain half is the part you change - this file builds for a Go repository
# because that is what NopsAI itself is.
#
#   docker build -t ghcr.io/acme/nopsai-codex-runner:1.0.0 .
#   docker push ghcr.io/acme/nopsai-codex-runner:1.0.0
#
# Then set container_image in both pipelines and in the shared checkout step to
# the tag you pushed. Pin it by digest for production use, the way the platform
# release pipeline pins its own images.
#
# No credentials belong in this image. NopsAI injects DEV_LOOP_GIT_TOKEN,
# DEV_LOOP_CODEX_API_KEY, and DEV_LOOP_NOPSAI_TOKEN as masked secrets at run
# time, resolved from the credential store.

FROM golang:1.26-bookworm

ARG CODEX_CLI_VERSION=0.20.0
ARG NODE_MAJOR=22

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash \
        ca-certificates \
        curl \
        git \
        jq \
        openssh-client \
    && curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN npm install --global "@openai/codex@${CODEX_CLI_VERSION}" \
    && npm cache clean --force

# Codex runs inside this container with approvals disabled, so the container is
# the isolation boundary. Keep it unprivileged and give it no access to
# anything the loop does not need.
RUN useradd --create-home --shell /bin/bash runner
WORKDIR /workspace
RUN chown runner:runner /workspace
USER runner

# Module and build caches live under the home directory so a warm image does
# not re-download the world on every task.
ENV GOFLAGS=-mod=mod \
    GOPATH=/home/runner/go \
    GOCACHE=/home/runner/.cache/go-build \
    npm_config_update_notifier=false

RUN git --version && codex --version && jq --version && go version
