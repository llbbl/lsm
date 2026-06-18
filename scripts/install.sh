#!/bin/sh
#
# lsm installer — downloads a per-OS release archive, verifies its sha256
# against checksums.txt, and installs the `lsm` binary onto your PATH.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/llbbl/lsm/main/scripts/install.sh | sh
#
# Environment overrides:
#   LSM_VERSION   Release tag to install (e.g. v0.1.3). Defaults to latest.
#   LSM_BIN       Install directory. Defaults to $HOME/.local/bin.
#
# Windows is not supported here — download the .zip from the releases page
# and follow the bundled INSTALL.md instead.

set -eu

REPO="llbbl/lsm"
BIN_DIR="${LSM_BIN:-$HOME/.local/bin}"

err() {
	printf 'install.sh: %s\n' "$1" >&2
	exit 1
}

info() {
	printf '==> %s\n' "$1"
}

need() {
	command -v "$1" >/dev/null 2>&1 || err "required tool '$1' not found on PATH"
}

# --- detect OS -------------------------------------------------------------

uname_s="$(uname -s)"
case "$uname_s" in
	Linux) os="linux" ;;
	Darwin) os="darwin" ;;
	*) err "unsupported OS '$uname_s' (this installer supports Linux and macOS; Windows users should use the .zip archive)" ;;
esac

# --- detect arch -----------------------------------------------------------

uname_m="$(uname -m)"
case "$uname_m" in
	x86_64 | amd64) arch="amd64" ;;
	arm64 | aarch64) arch="arm64" ;;
	*) err "unsupported architecture '$uname_m' (supported: x86_64/amd64, arm64/aarch64)" ;;
esac

# --- required tools --------------------------------------------------------

need uname
need tar

# Resolve a sha256 utility — Linux ships sha256sum, macOS ships shasum.
if command -v sha256sum >/dev/null 2>&1; then
	sha256_cmd="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
	sha256_cmd="shasum -a 256"
else
	err "no sha256 utility found (need 'sha256sum' or 'shasum')"
fi

# Resolve a downloader — prefer curl, fall back to wget.
if command -v curl >/dev/null 2>&1; then
	dl() { curl -fsSL "$1" -o "$2"; }
	dl_stdout() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
	dl() { wget -qO "$2" "$1"; }
	dl_stdout() { wget -qO - "$1"; }
else
	err "no downloader found (need 'curl' or 'wget')"
fi

# --- resolve version -------------------------------------------------------

if [ -n "${LSM_VERSION:-}" ]; then
	tag="$LSM_VERSION"
else
	info "Resolving latest release tag..."
	api_url="https://api.github.com/repos/${REPO}/releases/latest"
	# Pull the tag_name field from the JSON without requiring jq.
	tag="$(dl_stdout "$api_url" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
	[ -n "$tag" ] || err "could not resolve latest release tag from $api_url"
fi

archive="lsm-${tag}-${os}-${arch}.tar.gz"
base_url="https://github.com/${REPO}/releases/download/${tag}"

info "Installing lsm ${tag} (${os}/${arch})"

# --- download into a temp dir ----------------------------------------------

tmp="$(mktemp -d 2>/dev/null || mktemp -d -t lsm-install)"
[ -n "$tmp" ] && [ -d "$tmp" ] || err "could not create temp directory"
# shellcheck disable=SC2064
trap "rm -rf \"$tmp\"" EXIT INT TERM

info "Downloading ${archive}"
dl "${base_url}/${archive}" "${tmp}/${archive}" ||
	err "failed to download ${base_url}/${archive} (does a release exist for ${os}/${arch}?)"

info "Downloading checksums.txt"
dl "${base_url}/checksums.txt" "${tmp}/checksums.txt" ||
	err "failed to download checksums.txt"

# --- verify checksum -------------------------------------------------------

info "Verifying sha256 checksum"
expected="$(grep " ${archive}\$" "${tmp}/checksums.txt" | awk '{print $1}' | head -n 1)"
[ -n "$expected" ] || err "no checksum entry for ${archive} in checksums.txt"

actual="$(cd "$tmp" && $sha256_cmd "$archive" | awk '{print $1}')"
if [ "$expected" != "$actual" ]; then
	err "checksum mismatch for ${archive}: expected ${expected}, got ${actual}"
fi

# --- extract + install -----------------------------------------------------

info "Extracting"
tar xzf "${tmp}/${archive}" -C "$tmp"
[ -f "${tmp}/lsm" ] || err "archive did not contain an 'lsm' binary"

mkdir -p "$BIN_DIR"
mv "${tmp}/lsm" "${BIN_DIR}/lsm"
chmod +x "${BIN_DIR}/lsm"

info "Installed lsm to ${BIN_DIR}/lsm"

# --- post-install notes ----------------------------------------------------

case ":${PATH}:" in
	*":${BIN_DIR}:"*) ;;
	*)
		printf '\n'
		info "${BIN_DIR} is not on your PATH. Add this to your shell profile:"
		# Literal $PATH is intentional — this line is shown for the user to copy.
		# shellcheck disable=SC2016
		printf '\n    export PATH="%s:$PATH"\n' "$BIN_DIR"
		;;
esac

if [ "$os" = "darwin" ]; then
	printf '\n'
	info "macOS Gatekeeper: if launch is blocked, clear the quarantine flag:"
	printf '\n    xattr -d com.apple.quarantine %s/lsm\n' "$BIN_DIR"
fi

printf '\n'
info "Done. Run 'lsm --version' to confirm, then 'lsm init' to get started."
