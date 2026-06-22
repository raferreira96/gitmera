#!/bin/sh
# Installs the latest gitmera release into ~/.gitmera/bin and adds that
# directory to PATH in any shell startup files that already exist.
set -eu

REPO="raferreira96/gitmera"
INSTALL_DIR="$HOME/.gitmera/bin"
API_URL="https://api.github.com/repos/$REPO/releases/latest"
DOWNLOAD_BASE="https://github.com/$REPO/releases/download"

err() {
  echo "Error: $1" >&2
  exit 1
}

have() {
  command -v "$1" >/dev/null 2>&1
}

step() {
  echo "==> $1"
}

fetch() {
  # fetch URL OUTPUT_PATH
  if have curl; then
    curl -fsSL "$1" -o "$2"
  elif have wget; then
    wget -q "$1" -O "$2"
  else
    err "neither curl nor wget is available; install one of them and re-run this script"
  fi
}

fetch_stdout() {
  # fetch_stdout URL, prints the response body to stdout
  if have curl; then
    curl -fsSL "$1"
  elif have wget; then
    wget -qO- "$1"
  else
    err "neither curl nor wget is available; install one of them and re-run this script"
  fi
}

case "$(uname -s)" in
  Linux) OS="linux" ;;
  Darwin) OS="darwin" ;;
  *) err "unsupported operating system: $(uname -s). Download a binary manually from https://github.com/$REPO/releases" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) err "unsupported architecture: $(uname -m). Download a binary manually from https://github.com/$REPO/releases" ;;
esac

step "Looking up the latest gitmera release..."
TAG=$(fetch_stdout "$API_URL" | grep '"tag_name"' | head -n1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
  err "could not determine the latest release tag from $API_URL"
fi
VERSION=$(echo "$TAG" | sed -E 's/^v//')

ASSET="gitmera_${VERSION}_${OS}_${ARCH}.tar.gz"
ASSET_URL="$DOWNLOAD_BASE/$TAG/$ASSET"
CHECKSUMS_URL="$DOWNLOAD_BASE/$TAG/checksums.txt"

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

step "Downloading $ASSET ($TAG)..."
fetch "$ASSET_URL" "$WORKDIR/$ASSET"
fetch "$CHECKSUMS_URL" "$WORKDIR/checksums.txt"

step "Verifying checksum..."
EXPECTED=$(awk -v f="$ASSET" '$2 == f { print $1; exit }' "$WORKDIR/checksums.txt")
if [ -z "$EXPECTED" ]; then
  err "no checksum entry found for $ASSET in checksums.txt"
fi

if have sha256sum; then
  ACTUAL=$(sha256sum "$WORKDIR/$ASSET" | awk '{print $1}')
elif have shasum; then
  ACTUAL=$(shasum -a 256 "$WORKDIR/$ASSET" | awk '{print $1}')
elif have openssl; then
  ACTUAL=$(openssl dgst -sha256 "$WORKDIR/$ASSET" | awk '{print $NF}')
else
  err "no SHA-256 tool found (need sha256sum, shasum, or openssl)"
fi

if [ "$EXPECTED" != "$ACTUAL" ]; then
  err "checksum mismatch for $ASSET: expected $EXPECTED, got $ACTUAL"
fi

step "Installing to $INSTALL_DIR..."
mkdir -p "$INSTALL_DIR"
tar -xzf "$WORKDIR/$ASSET" -C "$WORKDIR"
mv "$WORKDIR/gitmera" "$INSTALL_DIR/gitmera"
chmod 755 "$INSTALL_DIR/gitmera"

PATH_LINE='export PATH="$HOME/.gitmera/bin:$PATH"'
MARK_START="# >>> gitmera installer >>>"
MARK_END="# <<< gitmera installer <<<"
TOUCHED=""

add_path_block() {
  # add_path_block FILE
  file="$1"
  if [ -f "$file" ] && grep -qF "$MARK_START" "$file" 2>/dev/null; then
    return
  fi
  {
    echo ""
    echo "$MARK_START"
    echo "$PATH_LINE"
    echo "$MARK_END"
  } >> "$file"
  TOUCHED="$TOUCHED $file"
}

step "Updating shell startup files..."
FOUND_RC=0
for rc in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile"; do
  if [ -f "$rc" ]; then
    FOUND_RC=1
    add_path_block "$rc"
  fi
done

if [ "$FOUND_RC" -eq 0 ]; then
  add_path_block "$HOME/.profile"
fi

echo ""
echo "gitmera $TAG installed to $INSTALL_DIR/gitmera"

if [ -n "$TOUCHED" ]; then
  echo "Added $INSTALL_DIR to PATH in:$TOUCHED"
else
  echo "$INSTALL_DIR was already on PATH in your shell startup files."
fi

echo ""
echo "This only takes effect in NEW terminal sessions. To use gitmera now, either"
echo "open a new terminal or run, for example:"
echo ""
echo "    source ~/.bashrc    # bash"
echo "    source ~/.zshrc     # zsh"
echo ""
echo "Then verify with: gitmera --version"
