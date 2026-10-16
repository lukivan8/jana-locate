#!/bin/sh
set -eu
# The URL is public browser configuration, not a secret. Restrict characters so
# environment values cannot inject executable JavaScript into config.js.
JANA_API_URL="${JANA_API_URL:-http://localhost:8080}"
case "$JANA_API_URL" in
  http://*|https://*) ;;
  *) echo 'JANA_API_URL must be an absolute HTTP(S) URL' >&2; exit 1 ;;
esac
case "$JANA_API_URL" in
  *[!a-zA-Z0-9:/._~%-]*) echo 'Unsupported character in JANA_API_URL' >&2; exit 1 ;;
esac
printf 'window.JANA_CONFIG = {"apiBaseUrl":"%s"};\n' "$JANA_API_URL" > /usr/share/nginx/html/config.js
