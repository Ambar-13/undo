#!/bin/sh
# undo quick install script
# Usage: curl -fsSL https://raw.githubusercontent.com/Ambar-13/undo/main/install.sh | sh

set -e

REPO="https://github.com/Ambar-13/undo"
INSTALL_DIR="${UNDO_INSTALL_DIR:-/usr/local/bin}"

echo "Building undo from source (requires Go 1.22+)..."

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

cd "$TMP"
git clone --depth=1 "$REPO" undo
cd undo
go build -o undo .

echo "Installing binary to $INSTALL_DIR..."
install -m 755 undo "$INSTALL_DIR/undo"

echo ""
echo "Run 'undo install' to hook into your shell."
echo "Then restart your shell or source your rc file."
