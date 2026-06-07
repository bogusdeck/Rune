#!/bin/bash

# Rune - Installation Script
# This script builds the Rune binary and moves it to a location in your PATH.

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Starting Rune installation...${NC}"

# 1. Check for Go
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go (1.25.3 or later) first."
    exit 1
fi

# 2. Build the binary
echo -e "${BLUE}Building binary...${NC}"
go build -o rune .

if [ ! -f ./rune ]; then
    echo "Error: Build failed."
    exit 1
fi

echo -e "${GREEN}Build successful!${NC}"

# 3. Installation Choice
echo ""
echo "Where would you like to install Rune?"
echo "1) System-wide (/usr/local/bin) - Recommended for most users (requires sudo)"
echo "2) Go bin folder (~/go/bin) - Standard for Go developers"
echo "3) Skip installation (keep binary in current folder)"
echo ""

read -p "Enter choice [1-3]: " choice

case $choice in
    1)
        echo -e "${BLUE}Installing to /usr/local/bin...${NC}"
        sudo mv rune /usr/local/bin/rune
        echo -e "${GREEN}Successfully installed! You can now type 'rune' from any directory.${NC}"
        ;;
    2)
        echo -e "${BLUE}Installing to ~/go/bin...${NC}"
        mkdir -p ~/go/bin
        mv rune ~/go/bin/rune
        echo -e "${GREEN}Successfully installed to ~/go/bin/rune${NC}"
        echo -e "Make sure your shell config (e.g., .zshrc or .bashrc) includes ~/go/bin in your PATH."
        ;;
    3)
        echo -e "Installation skipped. Binary is available at ./rune"
        ;;
    *)
        echo "Invalid choice. Binary remains at ./rune"
        ;;
esac

echo -e "${BLUE}Rune is ready to use.${NC}"
