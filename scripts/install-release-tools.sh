#!/usr/bin/env bash
set -euo pipefail

release_linux_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf '%s\n' amd64 ;;
    aarch64|arm64) printf '%s\n' arm64 ;;
    *) echo "Unsupported release tool architecture: $(uname -m)" >&2; return 1 ;;
  esac
}

release_tool_sha256() {
  local tool="$1"
  local version="$2"
  local arch="$3"
  local env_name=""
  case "$tool:$arch" in
    helm:amd64) env_name=NOPSAI_RELEASE_HELM_SHA256_AMD64 ;;
    helm:arm64) env_name=NOPSAI_RELEASE_HELM_SHA256_ARM64 ;;
    oras:amd64) env_name=NOPSAI_RELEASE_ORAS_SHA256_AMD64 ;;
    oras:arm64) env_name=NOPSAI_RELEASE_ORAS_SHA256_ARM64 ;;
    gh:amd64) env_name=NOPSAI_RELEASE_GH_SHA256_AMD64 ;;
    gh:arm64) env_name=NOPSAI_RELEASE_GH_SHA256_ARM64 ;;
    *) echo "Unsupported checksum target $tool linux/$arch" >&2; return 1 ;;
  esac

  local checksum="${!env_name:-}"
  if [[ -n "$checksum" ]]; then
    printf '%s\n' "$checksum"
    return
  fi

  case "$tool:$version:$arch" in
    helm:3.17.3:amd64) printf '%s\n' ee88b3c851ae6466a3de507f7be73fe94d54cbf2987cbaa3d1a3832ea331f2cd ;;
    helm:3.17.3:arm64) printf '%s\n' 7944e3defd386c76fd92d9e6fec5c2d65a323f6fadc19bfb5e704e3eee10348e ;;
    oras:1.2.3:amd64) printf '%s\n' b4efc97a91f471f323f193ea4b4d63d8ff443ca3aab514151a30751330852827 ;;
    oras:1.2.3:arm64) printf '%s\n' 90e24e234dc6dffe73365533db66fd14449d2c9ae77381081596bf92f40f6b82 ;;
    gh:2.74.2:amd64) printf '%s\n' c421091ae5800390e6aef1f50bfda59cc1d4f2ef2200bcd4e1a662c05c28c444 ;;
    gh:2.74.2:arm64) printf '%s\n' f0b07f0aeaf00f137df1bd33a76e717b1945f4b83bd6a3296b365414d3eb413f ;;
    *)
      echo "Checksum for $tool $version linux/$arch is not built in; set $env_name" >&2
      return 1
      ;;
  esac
}

verify_release_download() {
  local tool="$1"
  local version="$2"
  local arch="$3"
  local archive="$4"
  local checksum
  checksum="$(release_tool_sha256 "$tool" "$version" "$arch")"
  if [[ ! "$checksum" =~ ^[a-f0-9]{64}$ ]]; then
    echo "Invalid SHA-256 for $tool $version linux/$arch: $checksum" >&2
    return 1
  fi
  printf '%s  %s\n' "$checksum" "$archive" | sha256sum -c -
}

install_helm() {
  if command -v helm >/dev/null 2>&1; then
    return
  fi
  local version="${NOPSAI_RELEASE_HELM_VERSION:-3.17.3}"
  local arch
  arch="$(release_linux_arch)"
  local archive="/tmp/helm-v${version}-linux-${arch}.tar.gz"
  rm -rf "/tmp/linux-${arch}" "$archive"
  curl --proto '=https' --tlsv1.2 --retry 3 -fsSL "https://get.helm.sh/helm-v${version}-linux-${arch}.tar.gz" -o "$archive"
  verify_release_download helm "$version" "$arch" "$archive"
  tar -C /tmp -xzf "$archive"
  mv "/tmp/linux-${arch}/helm" /usr/local/bin/helm
  chmod +x /usr/local/bin/helm
}

install_oras() {
  if command -v oras >/dev/null 2>&1; then
    return
  fi
  local version="${NOPSAI_RELEASE_ORAS_VERSION:-1.2.3}"
  local arch
  arch="$(release_linux_arch)"
  local archive="/tmp/oras_${version}_linux_${arch}.tar.gz"
  rm -f /tmp/oras "$archive"
  curl --proto '=https' --tlsv1.2 --retry 3 -fsSL "https://github.com/oras-project/oras/releases/download/v${version}/oras_${version}_linux_${arch}.tar.gz" -o "$archive"
  verify_release_download oras "$version" "$arch" "$archive"
  tar -C /tmp -xzf "$archive" oras
  mv /tmp/oras /usr/local/bin/oras
  chmod +x /usr/local/bin/oras
}

install_gh() {
  if command -v gh >/dev/null 2>&1; then
    return
  fi
  local version="${NOPSAI_RELEASE_GH_VERSION:-2.74.2}"
  local arch
  arch="$(release_linux_arch)"
  local archive="/tmp/gh_${version}_linux_${arch}.tar.gz"
  rm -rf "/tmp/gh_${version}_linux_${arch}" "$archive"
  curl --proto '=https' --tlsv1.2 --retry 3 -fsSL "https://github.com/cli/cli/releases/download/v${version}/gh_${version}_linux_${arch}.tar.gz" -o "$archive"
  verify_release_download gh "$version" "$arch" "$archive"
  tar -C /tmp -xzf "$archive"
  mv "/tmp/gh_${version}_linux_${arch}/bin/gh" /usr/local/bin/gh
  chmod +x /usr/local/bin/gh
}
