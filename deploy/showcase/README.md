# Постоянный демонстрационный стенд

[Развёртывание Jana Locate](../../docs/deployment.md)

Стенд использует отдельный Compose-проект `jana-showcase`, собственный том и образы. API и статический frontend работают во внутренней сети без исходящего доступа. Только Cloudflare-коннектор подключён к внешней сети. Порты хоста не публикуются.

## Предварительная настройка

Создайте remotely managed named tunnel и proxied CNAME выбранного домена на `<tunnel UUID>.cfargotunnel.com`. Укажите ingress в таком порядке, заменив пример домена:

```yaml
ingress:
  - hostname: jana-locate.example.com
    path: ^/(v1(/.*)?|readyz|healthz|openapi\\.json)$
    service: http://backend:8080
  - hostname: jana-locate.example.com
    service: http://frontend:8080
  - service: http_status:404
```

Маршрутизация на шлюзе направляет запросы напрямую в независимые приложения. Frontend использует публичный HTTPS origin как URL API; backend-исходники, BFF и запросы при prerender не требуются.

```sh
mkdir -p .deploy
chmod 700 .deploy
cp deploy/showcase/env.example .deploy/showcase.env
# Укажите SHOWCASE_HOSTNAME и SHOWCASE_UID/GID из id -u / id -g.
# Сохраните токен коннектора одной строкой в .deploy/tunnel-token.
chmod 600 .deploy/showcase.env .deploy/tunnel-token
make showcase
make showcase-watch
```

Нужны Docker Engine/Compose с `!reset` и `build --provenance`, Bash, Python 3, Git, curl, flock и systemd user services. Совместимость Compose ранее проверялась с 5.1.3. Модели скачиваются при build; первая сборка дольше следующих.

Токен подключается как Docker secret из игнорируемого приватного каталога `.deploy/`. Не публикуйте его и не включайте в логи.

## Обновление и управление

`make showcase` собирает текущие исходники, обновляет контейнеры, удаляет устаревшие сервисы и проверяет healthchecks, публичный `/readyz`, HTML и URL API. Сборка заканчивается до замены контейнеров; при её ошибке предыдущие контейнеры продолжают работу. Во время замены возможен краткий перерыв — zero-downtime не гарантируется.

`make showcase-watch` запускает пользовательский сервис `jana-showcase.service`. Он проверяет хеш неигнорируемых файлов каждые 30 секунд, включая незакоммиченные и новые файлы, разворачивает изменения и повторяет неудачную попытку. Параллельные развёртывания сериализуются файловой блокировкой.

После смены конфигурации или токена выполните `make showcase`. После ротации токена перезапустите коннектор:

```sh
docker compose -p jana-showcase --env-file .deploy/showcase.env -f compose.yaml -f compose.showcase.yaml restart tunnel
```

```sh
make showcase-status
journalctl --user -u jana-showcase -f
./scripts/showcase.sh logs tunnel
make showcase-stop
# Возобновить:
make showcase
systemctl --user start jana-showcase
```

Restart policy восстанавливает контейнеры после запуска Docker. Для systemd без интерактивной сессии нужен user lingering (`loginctl enable-linger "$USER"`). Хост должен оставаться включённым и подключённым к сети: tunnel обеспечивает маршрутизацию, а не перенос приложения в облако.

## Данные и доступ

Остановка сохраняет том и Cloudflare-ресурсы. Не используйте `down --volumes`, если не намерены удалить рабочие данные стенда. Smoke-тесты используют отдельные временные тома.

API не имеет встроенной авторизации. На публичном стенде допустимы только синтетические демонстрационные данные. Для ограниченного доступа настройте Cloudflare Access. Геокодирование не входит в нормализацию.

[Named tunnels](https://developers.cloudflare.com/tunnel/get-started/) · [Файл токена](https://developers.cloudflare.com/tunnel/reference/run-parameters/).
