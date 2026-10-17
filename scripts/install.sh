#!/bin/sh
# Install published, versioned Docker images. No compilation or model preparation.
set -eu
umask 077
version=${JANA_VERSION:-0.1.0}
mode=${1:-api}
case "$mode" in api|web|full|hosted) ;; *) echo 'Режим: api, web, full или hosted' >&2; exit 2;; esac
case "$version" in ''|*[!a-zA-Z0-9._-]*) echo 'Некорректная версия' >&2; exit 2;; esac
command -v docker >/dev/null 2>&1 || { echo 'Установите Docker Engine или Docker Desktop.' >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo 'Для установки нужен curl.' >&2; exit 1; }
docker compose version >/dev/null
# Release files are pinned to a tag, never mixed with the latest working tree.
release="https://github.com/lukivan8/jana-locate/releases/download/v$version"
source="https://raw.githubusercontent.com/lukivan8/jana-locate/v$version"
install_dir=${JANA_INSTALL_DIR:-"$PWD/jana-locate"}
mkdir -p "$install_dir"
cd "$install_dir"
fetch() { curl --fail --location --retry 3 "$1" -o "$2.part" && mv "$2.part" "$2"; }
fetch "$source/deploy/compose.release.yaml" compose.yaml
fetch "$source/deploy/compose.release.host.yaml" compose.host.yaml
fetch "$source/scripts/jana" jana
chmod 700 jana
if [ -f .jana-mode ] && [ "$(cat .jana-mode)" != "$mode" ]; then ./jana down; fi
if [ ! -f .env ]; then
  project=${JANA_PROJECT_NAME:-jana-locate}
  case "$project" in ''|[!a-z0-9]*|*[!a-z0-9_-]*) echo 'Некорректное имя проекта.' >&2; exit 2;; esac
  token=$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')
  api_port=${JANA_API_PORT:-8080}
  web_port=${JANA_WEB_PORT:-3000}
  cli_port=${JANA_CLI_PORT:-8095}
  for port in "$api_port" "$web_port" "$cli_port"; do
    case "$port" in ''|*[!0-9]*) echo 'Порты должны быть числами.' >&2; exit 2;; esac
    [ "$port" -ge 1 ] && [ "$port" -le 65535 ] || exit 2
  done
  cat > .env <<ENV
JANA_VERSION=$version
COMPOSE_PROJECT_NAME=$project
API_PORT=$api_port
WEB_PORT=$web_port
CLI_PORT=$cli_port
JANA_BROWSER_API_URL=http://localhost:$api_port
CORS_ORIGINS=http://localhost:$web_port
JANA_WEB_TOKEN=$token
ENV
fi
# Preserve user configuration and credentials; only select the release version.
sed "s/^JANA_VERSION=.*/JANA_VERSION=$version/" .env > .env.next
mv .env.next .env
printf '%s\n' "$mode" > .jana-mode
rm -f .jana-host-network
if [ "$mode" = full ] || [ "$mode" = hosted ]; then
  network=${JANA_NETWORK_MODE:-auto}
  if [ "$network" = auto ]; then
    network=bridge
    if [ "$(uname -s)" = Linux ]; then
      engine_os=$(docker info --format '{{.OperatingSystem}}')
      case "$engine_os" in *'Docker Desktop'*) ;; *) network=host;; esac
    fi
  fi
  case "$network" in host) : > .jana-host-network;; bridge) ;; *) echo 'JANA_NETWORK_MODE: auto, host или bridge' >&2; exit 2;; esac
fi
sed '/^JANA_NORMALIZER_URL=/d' .env > .env.next
if [ "$mode" = hosted ]; then
  printf 'JANA_NORMALIZER_URL=https://jana.linker.kz\n' >> .env.next
elif [ -f .jana-host-network ]; then
  api_port=$(sed -n 's/^API_PORT=//p' .env)
  printf 'JANA_NORMALIZER_URL=http://127.0.0.1:%s\n' "$api_port" >> .env.next
fi
mv .env.next .env
load_image() {
  component=$1
  image="ghcr.io/lukivan8/jana-locate-$component:$version"
  if docker image inspect "$image" >/dev/null 2>&1; then return; fi
  printf 'Скачиваем готовый образ %s…\n' "$component"
  if docker pull --platform linux/amd64 "$image"; then return; fi
  # Public release archives provide anonymous installation even if registry access is restricted.
  archive="jana-locate-$component-$version-linux-amd64.tar.gz"
  fetch "$release/$archive" "$archive"
  fetch "$release/$archive.sha256" "$archive.sha256"
  if command -v sha256sum >/dev/null 2>&1; then sha256sum -c "$archive.sha256"
  else shasum -a 256 -c "$archive.sha256"; fi
  docker load -i "$archive"
  docker image inspect "$image" >/dev/null
  rm -f "$archive" "$archive.sha256"
}
case "$mode" in
  api) load_image api ;;
  web) load_image api; load_image web ;;
  full) load_image api; load_image cli ;;
  hosted) load_image cli ;;
esac
./jana up
printf '\nГотово. Каталог установки: %s\n' "$install_dir"
if [ "$mode" = hosted ]; then
  printf 'Нормализация: https://jana.linker.kz (адреса отправляются на hosted API).\n'
else
  api_port=$(sed -n 's/^API_PORT=//p' .env)
  printf 'Автономный API: http://localhost:%s\n' "$api_port"
fi
case "$mode" in
  web) web_port=$(sed -n 's/^WEB_PORT=//p' .env); printf 'Интерфейс: http://localhost:%s\n' "$web_port" ;;
  full|hosted) cli_port=$(sed -n 's/^CLI_PORT=//p' .env); printf 'Интерфейс: http://localhost:%s (пользователь jana, пароль JANA_WEB_TOKEN в .env).\n' "$cli_port" ;;
esac
printf 'Управление: cd "%s" && ./jana help\n' "$install_dir"
