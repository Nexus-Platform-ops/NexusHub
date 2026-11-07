#!/usr/bin/env bash
# Builds nexus-production.zip from the repository tree, excluding build
# artifacts and caches. Run from the repo root: bash build_zip.sh
set -euo pipefail
cd "$(dirname "$0")"

rm -f nexus-production.zip

zip -qr nexus-production.zip . \
  -x "*/.dart_tool/*" -x ".dart_tool/*" \
  -x "*/build/*" -x "build/*" \
  -x "*/.git/*" -x ".git/*" \
  -x "*.zip" \
  -x "*/coverage.out" \
  -x "*/.idea/*" -x "*/.vscode/*"

echo "nexus-production.zip created: $(du -h nexus-production.zip | cut -f1)"
unzip -l nexus-production.zip | tail -1
