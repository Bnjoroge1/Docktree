#!/usr/bin/env bash
set -euo pipefail

# Docktree installer
# Usage: curl -fsSL https://raw.githubusercontent.com/bnjoroge/docktree/main/install.sh | sh
#
# Options:
#   VERSION=v0.1.0  — override version (default: latest)
#   INSTALL_DIR     — override install directory (default: /usr/local/bin)

REPO="bnjoroge/docktree"
BINARY="docktree"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${VERSION:-latest}"

# --- colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BOLD='\033[1m'
RESET='\033[0m'

info()  { printf "${GREEN}✓${RESET} %s\n" "$1"; }
warn()  { printf "${YELLOW}!${RESET} %s\n" "$1"; }
error() { printf "${RED}✗${RESET} %s\n" "$1" >&2; exit 1; }

# --- detect platform ---
detect_platform() {
  local os arch

  case "$(uname -s)" in
    Linux*)   os="linux" ;;
    Darwin*)  os="darwin" ;;
    MINGW*|MSYS*|CYGWIN*) os="windows" ;;
    *) error "Unsupported OS: $(uname -s)" ;;
  esac

  case "$(uname -m)" in
    x86_64|amd64)   arch="amd64" ;;
    arm64|aarch64)   arch="arm64" ;;
    *) error "Unsupported architecture: $(uname -m)" ;;
  esac

  echo "${os}/${arch}"
}

# --- detect format ---
detect_format() {
  local platform="$1"
  case "$platform" in
    darwin/*|linux/*) echo "tar.gz" ;;
    windows/*)        echo "zip" ;;
  esac
}

# --- github api ---
github_api() {
  local url="$1"
  if command -v gh &>/dev/null; then
    gh api "$url" --jq '.tag_name' 2>/dev/null || true
  elif command -v curl &>/dev/null; then
    curl -fsSL -H "Accept: application/vnd.github+json" \
      "https://api.github.com/repos/${REPO}/releases/${VERSION}" 2>/dev/null \
      | grep '"tag_name"' | head -1 | cut -d '"' -f 4
  fi
}

# --- download ---
download() {
  local url="$1" dest="$2"
  if command -v curl &>/dev/null; then
    curl -fsSL -o "$dest" "$url"
  elif command -v wget &>/dev/null; then
    wget -qO "$dest" "$url"
  else
    error "Neither curl nor wget found. Install one and retry."
  fi
}

# --- main ---
main() {
  printf "\n${BOLD}Installing Docktree${RESET}\n\n"

  local platform
  platform="$(detect_platform)"
  local format
  format="$(detect_format "$platform")"
  local os="${platform%%/*}"
  local arch="${platform##*/}"

  info "Detected platform: ${BOLD}${platform}${RESET}"

  # resolve version
  if [ "$VERSION" = "latest" ]; then
    info "Fetching latest release..."
    VERSION="$(github_api "repos/${REPO}/releases/latest")"
    if [ -z "$VERSION" ]; then
      # fallback: try tags
      if command -v gh &>/dev/null; then
        VERSION="$(gh api "repos/${REPO}/tags" --jq '.[0].name' 2>/dev/null)"
      fi
    fi
    [ -z "$VERSION" ] && error "Could not determine latest version. Set VERSION manually."
  fi

  # strip leading v for archive naming if present
  local tag="$VERSION"
  local version="${tag#v}"

  info "Version: ${BOLD}${tag}${RESET}"

  # construct archive name
  local archive_name="${BINARY}_${version}_${os}_${arch}.${format}"
  local download_url="https://github.com/${REPO}/releases/download/${tag}/${archive_name}"

  info "Downloading ${archive_name}..."

  local tmpdir
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT

  download "$download_url" "${tmpdir}/${archive_name}"

  # extract
  info "Extracting..."
  case "$format" in
    tar.gz)
      tar -xzf "${tmpdir}/${archive_name}" -C "$tmpdir"
      ;;
    zip)
      unzip -qo "${tmpdir}/${archive_name}" -d "$tmpdir"
      ;;
  esac

  # find binary
  local bin_path
  bin_path="$(find "$tmpdir" -name "$BINARY" -type f | head -1)"
  if [ -z "$bin_path" ]; then
    error "Binary '${BINARY}' not found in archive"
  fi
  chmod +x "$bin_path"

  # install
  info "Installing to ${INSTALL_DIR}/${BINARY}..."
  mkdir -p "$INSTALL_DIR"

  # try direct copy, fall back to sudo
  if cp "$bin_path" "${INSTALL_DIR}/${BINARY}" 2>/dev/null; then
    :
  elif command -v sudo &>/dev/null; then
    sudo cp "$bin_path" "${INSTALL_DIR}/${BINARY}"
  else
    error "Cannot write to ${INSTALL_DIR}. Try: INSTALL_DIR=~/.local/bin $0"
  fi

  # verify
  if command -v "$BINARY" &>/dev/null; then
    local installed_version
    installed_version="$("$BINARY" --version 2>/dev/null || echo "$tag")"
    printf "\n${GREEN}${BOLD}✓ Docktree ${installed_version} installed successfully!${RESET}\n\n"
  else
    printf "\n${GREEN}${BOLD}✓ Docktree ${tag} installed to ${INSTALL_DIR}/${BINARY}${RESET}\n\n"
    warn "Add ${INSTALL_DIR} to your PATH if not already present:"
    warn "  export PATH=\"${INSTALL_DIR}:\$PATH\""
  fi
}

main "$@"
