FROM node:26-alpine@sha256:8996bb5821d62a8fd9bb27c1cb07462f3fdce6e47d640c6a881ead0b064ce216 AS build

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY . .
ARG VITE_API_BASE_URL
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}
ENV VITE_NOPSAI_VERSION=${VERSION}
ENV VITE_NOPSAI_COMMIT=${COMMIT}
ENV VITE_NOPSAI_BUILD_DATE=${BUILD_DATE}
RUN npm run build

FROM nginx:1.31-alpine@sha256:713fc2ebfd92e8217db451477182ee91944b123d2b1ee9fd5a8c6e91ca6c3a11

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

RUN rm /etc/nginx/conf.d/default.conf
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
COPY LICENSE THIRD_PARTY_NOTICES.md /usr/share/licenses/nopsai/

EXPOSE 80
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD wget -qO- http://localhost/ || exit 1
