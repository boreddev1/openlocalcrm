#!/usr/bin/env bash
# ==============================================================================
# OpenLocalCRM Universal Installer for macOS and Linux
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/boreddev1/openlocalcrm/main/scripts/install.sh | bash
#   curl -fsSL https://.../install.sh | bash -s -- install -y
# ==============================================================================

set -euo pipefail

# Visual colors
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}==============================================================================${NC}"
echo -e "${BLUE}  OpenLocalCRM — Cross-Platform Installer & Setup Launcher${NC}"
echo -e "${BLUE}==============================================================================${NC}"

# 1. Detect Operating System
RAW_OS="$(uname -s)"
case "${RAW_OS}" in
    Darwin*) OS="darwin" ;;
    Linux*)  OS="linux" ;;
    *)
        echo -e "${RED}❌ Nicht unterstütztes Betriebssystem: ${RAW_OS}${NC}"
        echo "Für Windows laden Sie bitte 'openlocalcrm-setup.exe' direkt herunter."
        exit 1
        ;;
esac

# 2. Detect CPU Architecture
RAW_ARCH="$(uname -m)"
case "${RAW_ARCH}" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)
        echo -e "${RED}❌ Nicht unterstützte Prozessor-Architektur: ${RAW_ARCH}${NC}"
        exit 1
        ;;
esac

BINARY_NAME="openlocalcrm-setup-${OS}-${ARCH}"
echo -e "Erkannt: ${GREEN}${OS} (${ARCH})${NC} -> ${BINARY_NAME}"

# 3. Determine Installation Destination
INSTALL_DIR="/usr/local/bin"
USE_SUDO=false

if [ ! -w "${INSTALL_DIR}" ]; then
    if [ "$(id -u)" -eq 0 ]; then
        # Already root
        USE_SUDO=false
    elif command -v sudo >/dev/null 2>&1; then
        USE_SUDO=true
    else
        # Fallback to user home directory ~/.local/bin
        INSTALL_DIR="${HOME}/.local/bin"
        mkdir -p "${INSTALL_DIR}"
        USE_SUDO=false
    fi
fi

TARGET_BIN="${INSTALL_DIR}/openlocalcrm"
TEMP_DOWNLOAD="$(mktemp -t openlocalcrm-install.XXXXXX)"
trap 'rm -f "${TEMP_DOWNLOAD}"' EXIT

# 4. Download Binary
REPO_URL="https://raw.githubusercontent.com/boreddev1/openlocalcrm/main/bin/${BINARY_NAME}"
RELEASE_URL="https://github.com/boreddev1/openlocalcrm/releases/latest/download/${BINARY_NAME}"

echo -e "Lade OpenLocalCRM Launcher herunter..."

if curl -fsSL -H "User-Agent: OpenLocalCRM-Installer" "${RELEASE_URL}" -o "${TEMP_DOWNLOAD}" 2>/dev/null; then
    echo -e "${GREEN}✅ Download von GitHub Releases erfolgreich.${NC}"
elif curl -fsSL -H "User-Agent: OpenLocalCRM-Installer" "${REPO_URL}" -o "${TEMP_DOWNLOAD}"; then
    echo -e "${GREEN}✅ Download aus Repository erfolgreich.${NC}"
else
    echo -e "${RED}❌ Download fehlgeschlagen. Bitte prüfen Sie Ihre Internetverbindung.${NC}"
    exit 1
fi

chmod +x "${TEMP_DOWNLOAD}"

# 5. Move to Install Directory
echo -e "Installiere Binärdatei nach ${INSTALL_DIR}/openlocalcrm..."
if [ "${USE_SUDO}" = true ]; then
    sudo mv "${TEMP_DOWNLOAD}" "${TARGET_BIN}"
    sudo chmod +x "${TARGET_BIN}"
else
    mv "${TEMP_DOWNLOAD}" "${TARGET_BIN}"
    chmod +x "${TARGET_BIN}"
fi

# Ensure ~/.local/bin is in PATH if installed there
if [ "${INSTALL_DIR}" = "${HOME}/.local/bin" ]; then
    case ":${PATH}:" in
        *:"${HOME}/.local/bin":*) ;;
        *)
            echo -e "${YELLOW}Hinweis: Fügen Sie '${HOME}/.local/bin' zu Ihrem PATH hinzu:${NC}"
            echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
            ;;
    esac
fi

echo -e "${GREEN}==============================================================================${NC}"
echo -e "${GREEN}  ✅ OpenLocalCRM CLI & GUI Launcher erfolgreich installiert!${NC}"
echo -e "${GREEN}==============================================================================${NC}"
"${TARGET_BIN}" --version || true

# 6. Execute forwarded arguments if any were provided
if [ "$#" -gt 0 ]; then
    echo -e "\nFühre Befehl aus: openlocalcrm $*"
    exec "${TARGET_BIN}" "$@"
else
    echo -e "\nNutzung:"
    echo -e "  ${BLUE}openlocalcrm${NC}            # Startet Web-GUI auf http://localhost:9099"
    echo -e "  ${BLUE}openlocalcrm install${NC}    # Startet geführte CRM-Installation im Terminal"
    echo -e "  ${BLUE}openlocalcrm --help${NC}     # Zeigt alle verfügbaren CLI-Befehle"
fi
