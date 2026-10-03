#!/bin/sh
# Installs the ai-flow CLI: detects OS/arch, downloads the release
# archive and checksums.txt from GitHub, verifies checksums.txt's
# keyless cosign signature (when cosign is on PATH, or always with -s),
# verifies the archive against its checksum, then installs the binary.
#
# Usage: install.sh [-v VERSION] [-d INSTALL_DIR] [-s]
#   VERSION     release tag, e.g. v0.1.0 (default: latest)
#   INSTALL_DIR where the binary is installed (default: /usr/local/bin,
#               or $HOME/.local/bin when /usr/local/bin is not writable)
#   -s          require the signature check: fail when cosign is not on
#               PATH instead of falling back to the checksum alone
#
# session-start.sh (ai-flow/hooks/session-start.sh) calls this script to
# install the pinned release named by the plugin manifest's cli_version
# when no binary is on PATH, or the one on PATH is out of range.

set -eu

REPO="pmi-ai-stack/ai-flow-cli"
VERSION=""
INSTALL_DIR=""
REQUIRE_SIGNATURE=0

while [ $# -gt 0 ]; do
	case "$1" in
	-v)
		VERSION="$2"
		shift 2
		;;
	-d)
		INSTALL_DIR="$2"
		shift 2
		;;
	-s)
		REQUIRE_SIGNATURE=1
		shift
		;;
	*)
		echo "install.sh: unknown argument: $1" >&2
		exit 2
		;;
	esac
done

die() {
	echo "install.sh: $1" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || die "$1 is required but not on PATH"
}

need curl
need tar

OS=""
case "$(uname -s)" in
Linux) OS="linux" ;;
Darwin) OS="darwin" ;;
*) die "unsupported OS: $(uname -s) (only linux and darwin are supported; Windows support is a known, tracked gap)" ;;
esac

ARCH=""
case "$(uname -m)" in
x86_64 | amd64) ARCH="amd64" ;;
arm64 | aarch64) ARCH="arm64" ;;
*) die "unsupported architecture: $(uname -m)" ;;
esac

if [ -z "$VERSION" ]; then
	VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
	[ -n "$VERSION" ] || die "could not determine the latest release; pass -v VERSION explicitly"
fi

if [ -z "$INSTALL_DIR" ]; then
	if [ -w /usr/local/bin ]; then
		INSTALL_DIR="/usr/local/bin"
	else
		INSTALL_DIR="${HOME}/.local/bin"
	fi
fi
mkdir -p "$INSTALL_DIR"

VERSION_NUM="${VERSION#v}"
ARCHIVE="ai-flow_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

echo "install.sh: downloading ${ARCHIVE} (${VERSION})" >&2
curl -fsSL -o "${WORK_DIR}/${ARCHIVE}" "${BASE_URL}/${ARCHIVE}" || die "download failed: ${BASE_URL}/${ARCHIVE}"
curl -fsSL -o "${WORK_DIR}/checksums.txt" "${BASE_URL}/checksums.txt" || die "download failed: ${BASE_URL}/checksums.txt"

# checksums.txt is signed keylessly by release.yml's own GitHub OIDC
# identity (see .goreleaser.yaml's signs:), so one verify-blob covers
# every archive the checksum step below then checks. The identity is
# pinned to this repository's release workflow at exactly this tag — a
# signature minted by any other workflow, repository or ref fails.
if command -v cosign >/dev/null 2>&1; then
	echo "install.sh: verifying checksums.txt signature" >&2
	curl -fsSL -o "${WORK_DIR}/checksums.txt.sig" "${BASE_URL}/checksums.txt.sig" || die "download failed: ${BASE_URL}/checksums.txt.sig"
	curl -fsSL -o "${WORK_DIR}/checksums.txt.pem" "${BASE_URL}/checksums.txt.pem" || die "download failed: ${BASE_URL}/checksums.txt.pem"
	cosign verify-blob \
		--certificate "${WORK_DIR}/checksums.txt.pem" \
		--signature "${WORK_DIR}/checksums.txt.sig" \
		--certificate-identity "https://github.com/${REPO}/.github/workflows/release.yml@refs/tags/${VERSION}" \
		--certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
		"${WORK_DIR}/checksums.txt" >&2 || die "signature verification of checksums.txt failed"
elif [ "$REQUIRE_SIGNATURE" -eq 1 ]; then
	die "cosign is required by -s but not on PATH; install it (https://docs.sigstore.dev/cosign/system_config/installation/) or drop -s"
else
	echo "install.sh: cosign not on PATH, skipping the signature check (checksum only; pass -s to require it)" >&2
fi

echo "install.sh: verifying checksum" >&2
(
	cd "$WORK_DIR"
	if command -v sha256sum >/dev/null 2>&1; then
		grep " ${ARCHIVE}\$" checksums.txt | sha256sum -c - || die "checksum verification failed"
	elif command -v shasum >/dev/null 2>&1; then
		grep " ${ARCHIVE}\$" checksums.txt | shasum -a 256 -c - || die "checksum verification failed"
	else
		die "neither sha256sum nor shasum is on PATH; cannot verify the checksum"
	fi
)

tar -xzf "${WORK_DIR}/${ARCHIVE}" -C "$WORK_DIR" ai-flow
chmod +x "${WORK_DIR}/ai-flow"
mv "${WORK_DIR}/ai-flow" "${INSTALL_DIR}/ai-flow"

echo "install.sh: installed ${INSTALL_DIR}/ai-flow (${VERSION})" >&2
"${INSTALL_DIR}/ai-flow" version --human >&2 || true
