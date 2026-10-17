#!/bin/bash
# Dedicated Docker deployment; source edits never run inside the served containers.
set -euo pipefail
cd "$(dirname "$0")/.."
repo=$PWD
state="$repo/.deploy"
mkdir -p "$state"
chmod 700 "$state"
compose() {
  docker compose -p jana-showcase --env-file "$state/showcase.env" \
    -f compose.yaml -f compose.showcase.yaml "$@"
}
fingerprint() {
  # Includes new uncommitted source files; ignores secrets, models, and build output
  # according to the repository's ignore rules. Names and content both matter.
  python3 - <<'PY'
import hashlib, pathlib, subprocess
h = hashlib.sha256()
names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard'])
for name in sorted(set(names.split(b'\0')) - {b''}):
    p = pathlib.Path(name.decode())
    if p.is_file():
        h.update(name + b'\0')
        h.update(p.read_bytes())
print(h.hexdigest())
PY
}
deploy() (
  exec 9>"$state/deploy.lock"
  flock 9
  test -s "$state/showcase.env" && test -s "$state/tunnel-token" || {
    echo 'Configure .deploy/showcase.env and .deploy/tunnel-token first.' >&2; exit 1;
  }
  before=$(fingerprint)
  compose config --quiet
  # Finish every build before replacing running containers.
  # Avoid timestamped attestations changing image identity for cached services.
  compose build --provenance=false
  compose up -d --remove-orphans --wait --wait-timeout 300
  hostname=$(compose config --format json | python3 -c 'import json,sys; print(json.load(sys.stdin)["services"]["frontend"]["environment"]["JANA_API_URL"])')
  curl --fail --silent --show-error --retry 20 --retry-all-errors --retry-delay 3 \
    --connect-timeout 10 --max-time 30 "$hostname/readyz" >/dev/null
  curl --fail --silent --show-error --connect-timeout 10 --max-time 30 "$hostname/" >/dev/null
  curl --fail --silent --show-error --connect-timeout 10 --max-time 30 "$hostname/config.js" \
    | python3 -c 'import sys; assert sys.argv[1] in sys.stdin.read(), "Public API configuration does not match showcase origin"' "$hostname"
  # If files changed during the build, the next poll deploys those edits too.
  printf '%s\n' "$before" > "$state/deployed.sha256"
  date -u +%FT%TZ > "$state/deployed-at"
  echo "Showcase ready: $hostname"
)
case "${1:-deploy}" in
  deploy) deploy ;;
  watch)
    while true; do
      if [[ $(fingerprint) != $(cat "$state/deployed.sha256" 2>/dev/null || true) ]]; then
        "$repo/scripts/showcase.sh" deploy || echo 'Showcase deployment failed; will retry. See preceding build output.' >&2
      fi
      sleep "${SHOWCASE_POLL_SECONDS:-30}"
    done ;;
  status) compose ps; test ! -f "$state/deployed-at" || cat "$state/deployed-at" ;;
  logs) compose logs --tail 100 "${@:2}" ;;
  stop) compose stop ;;
  *) echo 'Usage: scripts/showcase.sh {deploy|watch|status|logs [service]|stop}' >&2; exit 2 ;;
esac
