# Установка готовых контейнеров

[Документация](README.md) · [Подключение API](integration.md)

## Попробовать онлайн

**[Откройте Jana Locate → jana.linker.kz](https://jana.linker.kz)**. Для знакомства с приложением ничего устанавливать не нужно. Можно также вызывать hosted API по `https://jana.linker.kz/v1/normalize`.

## Только API: одна команда Docker

Для нормализатора достаточно установленного Docker:

```sh
docker run -d --name jana-locate --platform linux/amd64 \
  --restart unless-stopped -p 127.0.0.1:8080:8080 \
  -v jana-locate-api-data:/data \
  ghcr.io/lukivan8/jana-locate-api:0.1.0
```

Docker автоматически скачает образ. API — `http://localhost:8080`, готовность — `/readyz`. Модели и справочники уже внутри. Остановить: `docker stop jana-locate`; снова запустить: `docker start jana-locate`. Данные остаются в томе `jana-locate-api-data`.

Ниже — альтернативный установщик с Compose и управлением через `./jana`. Выберите один путь: одновременный запуск двух экземпляров на порту 8080 вызовет конфликт.

## Установить локально одной командой

Нужны Docker Engine/Docker Desktop с Docker Compose v2, curl и POSIX shell (Linux, macOS или WSL). Go, Node.js, Python, Git и подготовка моделей не требуются. Публикуются образы `linux/amd64`; ARM-хостам нужна поддержка эмуляции amd64 в Docker. Нативный ARM пока не проверен и не выпускается.

Выберите **один** комплект:

| Комплект | Что запускается | Адрес |
| --- | --- | --- |
| `api` | Автономный API | `http://localhost:8080` |
| `web` | Автономный API и отдельный веб-интерфейс | `http://localhost:3000` |
| `full` | Автономный API, CLI и встроенный веб-интерфейс | `http://localhost:8095` |
| `hosted` | Локальный CLI/интерфейс, нормализация через jana.linker.kz | `http://localhost:8095` |

```sh
curl -fsSL https://raw.githubusercontent.com/lukivan8/jana-locate/main/scripts/install.sh | sh -s -- api
```

Замените последний аргумент на `web`, `full` или `hosted`. Установщик скачивает опубликованные образы, создаёт `./jana-locate/` и запускает Docker Compose **без сборки исходников**. [Исходник установщика](../scripts/install.sh).

Изображения скачиваются из GHCR; если реестр недоступен без авторизации, установщик использует публичные архивы того же [релиза 0.1.0](https://github.com/lukivan8/jana-locate/releases/tag/v0.1.0) и проверяет SHA-256. Учётная запись GitHub для этого пути не нужна.

Скачивание API занимает несколько ГБ: в образ включены модели libpostal и справочники. Первый старт загружает их в память; ожидайте готовности. API ограничен 3 GiB RAM и двумя CPU в Compose. Для полного комплекта нужен дополнительный запас памяти Docker и свободного диска. Эти значения — настройки контейнеров, а не измеренные минимальные требования.

## Автономность

**Комплекты `api`, `web`, `full` полностью автономны после скачивания.** Парсер, модели, справочники и сохранение рабочих данных находятся локально. Runtime не загружает модели и не обращается к hosted API, онлайн-геокодерам или внешней LLM. Запуск `./jana up` использует только уже установленные образы.

Режим `hosted` выбран явно: он требует интернета и отправляет адресные строки на `https://jana.linker.kz`. Пароли вашей БД туда не передаются. [Подключение к обоим вариантам](integration.md#локальный-и-hosted-api).

## Проверить API

Для автономного комплекта:

```sh
curl --fail-with-body http://localhost:8080/readyz
curl --fail-with-body http://localhost:8080/v1/normalize \
  -H 'Content-Type: application/json' \
  -d '{"records":[{"id":"order-1","raw_address":"Астана, проспект Туран 55/6 кв 99"}]}'
```

HTTP 200 означает обработку запроса. Для каждой записи проверьте `status`, `warnings` и неизвестные поля. Геокодирование не выполняется; legacy-поля координат остаются `null`.

## Управлять установкой

```sh
cd jana-locate
./jana status
./jana logs
./jana down
./jana up
```

Остановка сохраняет Docker-тома. `./jana compose down --volumes` удаляет данные — для обычной остановки используйте `./jana down`. Настройки хранятся в `.env` с ограниченными правами. В `full`/`hosted` вход на сайт: пользователь `jana`, пароль — значение `JANA_WEB_TOKEN` в `.env`.

Для CLI:

```sh
./jana connect
./jana normalize
./jana normalize --help
```

База PostgreSQL устанавливается отдельно. На обычном Linux CLI использует host network и может подключиться к `localhost` вашей базы. Docker Desktop использует bridge: для базы на компьютере укажите `host.docker.internal`. `JANA_NETWORK_MODE=bridge` позволяет выбрать bridge явно до установки.

## Другие порты и каталог

Переменные задаются до запуска установщика. При передаче через pipe они должны относиться к `sh`, например:

```sh
curl -fsSL https://raw.githubusercontent.com/lukivan8/jana-locate/main/scripts/install.sh | \
  JANA_INSTALL_DIR="$HOME/jana-test" JANA_PROJECT_NAME=jana-test JANA_API_PORT=18080 JANA_WEB_PORT=13000 sh -s -- web
```

Для нескольких независимых установок задавайте разные `JANA_PROJECT_NAME`, каталоги и порты. Для полного комплекта порт сайта задаёт `JANA_CLI_PORT`. Повторная установка сохраняет `.env` и данные; смена комплекта останавливает прежний комплект, не удаляя тома. Порты, браузерный URL API и CORS после установки меняются в `.env` согласованно.

## Загрузка в закрытую сеть

Скачайте нужные архивы `jana-locate-<api|web|cli>-0.1.0-linux-amd64.tar.gz` и их `.sha256` из [релиза](https://github.com/lukivan8/jana-locate/releases/tag/v0.1.0). Перенесите их, файлы Compose, launcher и `.env` в закрытую сеть. Проверьте SHA-256, выполните `docker load -i <архив>` для каждого образа и запустите `./jana up`. Интернет для работы локальных комплектов не нужен. [Подробности](deployment.md).

## Сборка из исходников — для разработки

Это дополнительный путь, а не обязательный этап установки:

```sh
git clone https://github.com/lukivan8/jana-locate.git
cd jana-locate
cp .env.example .env
docker compose up --build --wait
```

Go 1.23+, Node.js 22.12+ и Python 3 нужны только для разработки вне контейнеров. [Backend](../backend/README.md) · [Frontend](../frontend/README.md) · [CLI](../cli/README.md).
