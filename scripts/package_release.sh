#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${1:-${RELEASE_TAG:-v1.0.0}}"
RELEASE_DIR="${ROOT_DIR}/build/release"
APP_PATH="${ROOT_DIR}/build/SYNTRIX.app"

if [[ ! -d "${APP_PATH}" ]]; then
  echo "SYNTRIX.app not found at ${APP_PATH}; building bundle first..."
  make -C "${ROOT_DIR}" bundle
fi

rm -rf "${RELEASE_DIR}"
mkdir -p "${RELEASE_DIR}"

ZIP_VERSIONED="${RELEASE_DIR}/SYNTRIX-macOS-${VERSION}.zip"
ZIP_LATEST="${RELEASE_DIR}/SYNTRIX-macOS.zip"
DMG_VERSIONED="${RELEASE_DIR}/SYNTRIX-macOS-${VERSION}.dmg"
DMG_LATEST="${RELEASE_DIR}/SYNTRIX-macOS.dmg"

echo "Packaging ${ZIP_VERSIONED}..."
ditto -c -k --sequesterRsrc --keepParent "${APP_PATH}" "${ZIP_VERSIONED}"
cp "${ZIP_VERSIONED}" "${ZIP_LATEST}"

echo "Packaging ${DMG_VERSIONED}..."
STAGING_DIR="$(mktemp -d)"
trap 'rm -rf "${STAGING_DIR}"' EXIT
ditto "${APP_PATH}" "${STAGING_DIR}/SYNTRIX.app"
ln -s /Applications "${STAGING_DIR}/Applications"
hdiutil create \
  -volname "SYNTRIX ${VERSION}" \
  -srcfolder "${STAGING_DIR}" \
  -ov \
  -format UDZO \
  "${DMG_VERSIONED}"
cp "${DMG_VERSIONED}" "${DMG_LATEST}"

(
  cd "${RELEASE_DIR}"
  shasum -a 256 SYNTRIX-macOS*.zip SYNTRIX-macOS*.dmg > SHA256SUMS.txt
)

echo "Release artifacts created in ${RELEASE_DIR}:"
ls -lh "${RELEASE_DIR}"
