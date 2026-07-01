#!/usr/bin/env bash
set -euo pipefail

# Vela Universal Installer
# Usage: curl -sSL https://raw.githubusercontent.com/Syfra3/vela/main/scripts/install-vela.sh | bash

VERSION="${VELA_VERSION:-latest}"
INSTALL_DIR="${VELA_INSTALL_DIR:-/usr/local/bin}"
REPO="Syfra3/vela"
BINARY_NAME="vela"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}==>${NC} $1" >&2
}

log_success() {
    echo -e "${GREEN}✓${NC} $1" >&2
}

log_warn() {
    echo -e "${YELLOW}!${NC} $1" >&2
}

log_error() {
    echo -e "${RED}✗${NC} $1" >&2
}

detect_platform() {
    local os=""
    local arch=""

    case "$(uname -s)" in
        Linux*) os="linux" ;;
        Darwin*) os="darwin" ;;
        MINGW*|MSYS*|CYGWIN*) os="windows" ;;
        *)
            log_error "Unsupported operating system: $(uname -s)"
            exit 1
            ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)
            log_error "Unsupported architecture: $(uname -m)"
            exit 1
            ;;
    esac

    echo "${os}-${arch}"
}

get_latest_version() {
    log_info "Fetching latest version..."
    local latest
    latest=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$latest" ]; then
        log_error "Failed to fetch latest release"
        exit 1
    fi

    echo "$latest"
}

normalize_version() {
    local raw="$1"

    if [ "$raw" = "latest" ]; then
        get_latest_version
        return
    fi

    if [[ "$raw" == v* ]]; then
        echo "$raw"
        return
    fi

    echo "v${raw}"
}

download_and_install() {
    local version="$1"
    local platform="$2"
    local tmpdir
    tmpdir=$(mktemp -d)

    local version_number="${version#v}"
    local filename="${BINARY_NAME}-${version_number}-${platform}.tar.gz"
    local checksum_file="${filename}.sha256"
    local base_url="https://github.com/${REPO}/releases/download/${version}"
    local url="${base_url}/${filename}"
    local checksum_url="${base_url}/${checksum_file}"

    log_info "Downloading ${BINARY_NAME} ${version} for ${platform}..."
    log_info "URL: ${url}"

    if ! curl -fL -o "${tmpdir}/${filename}" "${url}"; then
        log_error "Download failed"
        rm -rf "$tmpdir"
        exit 1
    fi

    if curl -fsSL -o "${tmpdir}/${checksum_file}" "${checksum_url}"; then
        log_info "Verifying checksum..."
        if command -v sha256sum &>/dev/null; then
            (cd "$tmpdir" && sha256sum -c "$checksum_file")
        elif command -v shasum &>/dev/null; then
            (cd "$tmpdir" && shasum -a 256 -c "$checksum_file")
        else
            log_warn "No sha256 verifier found; skipping checksum verification"
        fi
    else
        log_warn "Checksum file unavailable; skipping checksum verification"
    fi

    log_info "Extracting archive..."
    if ! tar -xzf "${tmpdir}/${filename}" -C "$tmpdir"; then
        log_error "Extraction failed"
        rm -rf "$tmpdir"
        exit 1
    fi

    local binary_file="${BINARY_NAME}"
    if [ "$platform" = "windows-amd64" ]; then
        binary_file="${BINARY_NAME}.exe"
    fi

    if [ ! -f "${tmpdir}/${binary_file}" ]; then
        log_error "Binary not found in archive"
        rm -rf "$tmpdir"
        exit 1
    fi

    log_info "Installing to ${INSTALL_DIR}..."
    if [ ! -d "$INSTALL_DIR" ]; then
        log_warn "Install directory doesn't exist. Attempting to create: ${INSTALL_DIR}"
        if ! mkdir -p "$INSTALL_DIR" 2>/dev/null; then
            log_error "Failed to create ${INSTALL_DIR}. Try running with sudo or set VELA_INSTALL_DIR to a writable location."
            log_info "Example: export VELA_INSTALL_DIR=\$HOME/.local/bin"
            rm -rf "$tmpdir"
            exit 1
        fi
    fi

    if ! mv "${tmpdir}/${binary_file}" "${INSTALL_DIR}/${binary_file}" 2>/dev/null; then
        log_error "Permission denied. Try running with sudo or set VELA_INSTALL_DIR to a writable location."
        log_info "Example: export VELA_INSTALL_DIR=\$HOME/.local/bin"
        rm -rf "$tmpdir"
        exit 1
    fi

    chmod +x "${INSTALL_DIR}/${binary_file}"
    rm -rf "$tmpdir"

    log_success "${BINARY_NAME} ${version} installed successfully!"
}

verify_installation() {
    log_info "Verifying installation..."

    if ! command -v "${BINARY_NAME}" &>/dev/null; then
        log_warn "${BINARY_NAME} is installed but not in PATH"
        log_info "Add ${INSTALL_DIR} to your PATH:"
        log_info "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        return
    fi

    local installed_version
    installed_version=$("${BINARY_NAME}" version 2>&1 | head -n1 || echo "unknown")
    log_success "Installation verified: ${installed_version}"
}

print_next_steps() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${GREEN}Vela installed successfully!${NC}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Get started:"
    echo "  $ vela build ./my-repo"
    echo "  $ vela status --graph ./my-repo/.vela/graph.json"
    echo ""
    echo "Documentation: https://github.com/${REPO}"
    echo ""
}

main() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${BLUE}Vela Installer${NC}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    for cmd in curl tar grep sed; do
        if ! command -v "$cmd" &>/dev/null; then
            log_error "Required command not found: $cmd"
            exit 1
        fi
    done

    if ! command -v sha256sum &>/dev/null && ! command -v shasum &>/dev/null; then
        log_warn "No sha256 verifier found; checksum verification will be skipped"
    fi

    local platform
    platform=$(detect_platform)
    log_info "Detected platform: ${platform}"

    VERSION=$(normalize_version "$VERSION")
    log_info "Target version: ${VERSION}"

    download_and_install "$VERSION" "$platform"
    verify_installation
    print_next_steps
}

main "$@"
