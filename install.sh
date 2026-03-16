#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Detect OS and Architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case $ARCH in
    x86_64|amd64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

# Determine binary name
BINARY_NAME="dockrouter-${OS}-${ARCH}"
if [ "$OS" = "windows" ]; then
    BINARY_NAME="${BINARY_NAME}.exe"
fi

# Get latest version
echo -e "${GREEN}Fetching latest version...${NC}"
LATEST_VERSION=$(curl -s https://api.github.com/repos/DockRouter/dockrouter/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST_VERSION" ]; then
    echo -e "${YELLOW}Could not determine latest version, using 'latest'${NC}"
    LATEST_VERSION="latest"
fi

echo -e "${GREEN}Installing DockRouter ${LATEST_VERSION}...${NC}"

# Download URL
if [ "$LATEST_VERSION" = "latest" ]; then
    DOWNLOAD_URL="https://github.com/DockRouter/dockrouter/releases/latest/download/${BINARY_NAME}"
else
    DOWNLOAD_URL="https://github.com/DockRouter/dockrouter/releases/download/${LATEST_VERSION}/${BINARY_NAME}"
fi

# Download binary
echo -e "${GREEN}Downloading from ${DOWNLOAD_URL}...${NC}"
curl -sL "$DOWNLOAD_URL" -o dockrouter

# Make executable
chmod +x dockrouter

# Install
INSTALL_DIR="/usr/local/bin"
if [ -w "$INSTALL_DIR" ]; then
    mv dockrouter "$INSTALL_DIR/dockrouter"
    echo -e "${GREEN}Installed to ${INSTALL_DIR}/dockrouter${NC}"
else
    echo -e "${YELLOW}Need sudo to install to ${INSTALL_DIR}${NC}"
    sudo mv dockrouter "$INSTALL_DIR/dockrouter"
    echo -e "${GREEN}Installed to ${INSTALL_DIR}/dockrouter${NC}"
fi

# Verify installation
echo -e "${GREEN}Verifying installation...${NC}"
dockrouter --version || echo "DockRouter installed successfully!"

# Print usage
echo ""
echo -e "${GREEN}╔═══════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║              DockRouter Installed Successfully!           ║${NC}"
echo -e "${GREEN}╚═══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "Quick Start:"
echo ""
echo "  # Run with Docker socket"
echo "  dockrouter --docker-socket /var/run/docker.sock"
echo ""
echo "  # Or with Docker"
echo "  docker run -d \\"
echo "    -p 80:80 -p 443:443 -p 9090:9090 \\"
echo "    -v /var/run/docker.sock:/var/run/docker.sock:ro \\"
echo "    dockrouter/dockrouter:latest"
echo ""
echo "Documentation: https://github.com/DockRouter/dockrouter"
echo ""
