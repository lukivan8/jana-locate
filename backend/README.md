# Jana Locate — HTTP API

Самостоятельный Go-сервис нормализации адресов Казахстана. Этот каталог содержит всё для сборки API: исходники, OpenAPI, справочники, Dockerfile, SDK и тесты. Интерфейс и CLI для работы API не обязательны.

**[Основная страница инструмента](../README.md)** · [Подключение своего сервиса](../docs/integration.md) · [Контракт](api/openapi.json)

## Установка готового API

Основной способ локальной работы — [скачать и запустить готовый контейнер одной командой Docker](../docs/quickstart.md#только-api-одна-команда-docker). Для API достаточно Docker; единый установщик с Compose доступен как альтернативный путь. Он не требует Go, Python, Git или сборки моделей. API полностью автономен после скачивания: libpostal и справочник входят в образ.

Если локальный API не нужен, используйте `https://jana.linker.kz/v1/normalize` с тем же контрактом. [Локальная и hosted-интеграция](../docs/integration.md#локальный-и-hosted-api).

## Запуск из исходников — для разработки

Из корня Jana Locate:

```sh
make reference
cd backend
REFERENCE_PATH="$PWD/data/reference/reference.sqlite" go run ./cmd/server
```

Нужны Go 1.23+ и Python 3 для подготовки SQLite. Без native libpostal этот запуск использует резервный разбор и возвращает `libpostal_unavailable`. Для полного режима из корня проекта выполните `docker compose up --build --wait backend`.

## Самостоятельная сборка только этого каталога

```sh
docker build -t jana-address-api:local .
docker run --rm -p 127.0.0.1:8080:8080 \
  -v jana-locate-api-data:/data jana-address-api:local
```

Dockerfile не читает соседние каталоги. libpostal и справочник готовятся при сборке; отдельного Python/GLiNER-сервиса в runtime нет. Первая сборка требует сети и нескольких ГБ диска. Контейнер работает от UID/GID 10001; `/data` содержит рабочие данные.

## Контракт и поведение

- `POST /v1/normalize` — синхронная нормализация, без сохранения задания.
- `GET /healthz`, `GET /readyz` — состояние процесса и готовность.
- `GET /openapi.json` — встроенный контракт.
- `/v1/normalization-jobs` — отдельный процесс сохраняемой проверки.
- `/v1/normalize/candidates` и `/v1/normalize/selections` — разрешение неоднозначности в собственном журнале потребителя.
- Legacy `/v1/catalog` и `/v1/catalog/confirmations` независимы от нормализации.

Геокодирование не выполняется. Новые результаты имеют `null` в `coordinates`, `catalog_id`, `catalog_revision`. Исходный ввод и порядок сохраняются. Сильная неоднозначность не скрывается; `confidence` не является измеренной точностью.

[Поля, лимиты, ошибки и выбор населённого пункта](docs/consumer-guide.md) · [Задания](docs/jobs.md) · [Алгоритм](docs/normalization.md) · [Справочник](docs/reference.md).

## Go SDK и совместимость

`pkg/api` и `pkg/client` — публичные пакеты. Исторический module path `github.com/lukivan8/jana-address` сохранён; актуальные исходники находятся в Jana Locate. Старая самостоятельная публикация не гарантирует соответствие текущему API. Для внешней интеграции без зависимости от module path используйте [HTTP-пример Go](../examples/http/go/main.go).

SDK использует стандартный HTTP-клиент. Таймаут по умолчанию 60 секунд; для больших пакетов его нужно подобрать отдельно. `*client.HTTPError` содержит HTTP-статус и ошибку API. [Локальный компилируемый SDK-пример](examples/consumer/main.go).

## Разработка

Из этого каталога:

```sh
make check       # race-тесты, vet и сборка
make generate    # DTO из api/openapi.json
make reference   # подготовка SQLite
```

Сначала изменяйте спецификацию, затем генерируйте DTO. При работе из полного репозитория используйте корневые `make generate` и `make check`: они обновляют принятые снимки frontend/CLI.

## Эксплуатация

API не имеет встроенной авторизации. Для внешнего доступа нужен защищённый шлюз. CORS не заменяет аутентификацию. Один процесс владеет одним рабочим файловым хранилищем. Не направляйте тесты на рабочие данные.

[Настройки и хранение](../docs/deployment.md) · [Legacy-каталог](docs/catalog.md) · [Безопасность](../SECURITY.md) · [Источники и атрибуция](../THIRD_PARTY_NOTICES.md).

Код — [MIT](../LICENSE). Условия сторонних моделей, библиотек и данных сохраняются отдельно.
