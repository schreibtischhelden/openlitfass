#!/bin/sh
# Refreshes the vendored copy of markdown-pages.js in web/vendor/markdown-pages/.
# It renders the Markdown pages of an instance (seite.html, see
# web/templates/page.html) with the libraries it loads itself: Showdown,
# DOMPurify and showdown-forms. Pico CSS, highlight.js and the editor are
# not copied.
#
#   scripts/vendor-markdown-pages.sh            the pinned release below
#   scripts/vendor-markdown-pages.sh v2.1.0     another release
#
# After a version change, update the pinned version here and the versions
# in web/THIRD-PARTY-NOTICES.txt (see web/vendor/markdown-pages/VERSION).
set -eu

VERSION="${1:-v2.0.0}"
REPO="schreibtischhelden/markdown-pages.js"
BASE="https://raw.githubusercontent.com/$REPO/$VERSION"

root="$(cd "$(dirname "$0")/.." && pwd)"
dest="$root/web/vendor/markdown-pages"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

files="
LICENSE
markdown-pages.js
assets/vendor/versions.json
assets/vendor/showdown/LICENSE
assets/vendor/showdown/showdown.min.js
assets/vendor/dompurify/LICENSE
assets/vendor/dompurify/purify.min.js
assets/vendor/showdown-forms/LICENSE
assets/vendor/showdown-forms/showdown-forms.js
"

for f in $files; do
	mkdir -p "$tmp/$(dirname "$f")"
	curl -fsSL --proto '=https' "$BASE/$f" -o "$tmp/$f"
done
echo "$REPO $VERSION" > "$tmp/VERSION"

# Replace the whole directory, so files dropped upstream do not linger.
rm -rf "$dest"
mkdir -p "$(dirname "$dest")"
cp -R "$tmp" "$dest"
chmod -R u+rwX,go+rX "$dest"
echo "markdown-pages.js $VERSION -> web/vendor/markdown-pages/"
cat "$dest/assets/vendor/versions.json"
