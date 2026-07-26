FROM node:22-alpine AS build

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

FROM nginx:1.27-alpine

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG SOURCE_URL=https://github.com/nopsai/nopsai

LABEL org.opencontainers.image.version="${VERSION}" \
  org.opencontainers.image.revision="${COMMIT}" \
  org.opencontainers.image.created="${BUILD_DATE}" \
  org.opencontainers.image.source="${SOURCE_URL}"

RUN rm /etc/nginx/conf.d/default.conf
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD wget -qO- http://localhost/ || exit 1
