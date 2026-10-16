# Jana Address

Go-сервис нормализации адресов Казахстана и сопоставления с каталогом зданий. Принимает произвольные адресные строки, выделяет компоненты, сохраняет исходный ввод и возвращает координаты только при однозначном совпадении с известным зданием.

Репозиторий содержит самостоятельный Go-модуль, HTTP API, OpenAPI-контракт, Go SDK, тесты и Dockerfile. Веб-приложение и Python-сервис здесь не требуются для сборки. Полный комплект с интерфейсом и GLiNER доступен в [jana-locate](https://github.com/lukivan8/jana-locate).

> Три здания в начальном каталоге и их координаты — синтетические примеры. По умолчанию они отключены. Сервис не является полноценным геокодером всей страны: неизвестные координаты возвращаются как `null`, а неоднозначные результаты требуют проверки. `confidence` — эвристическая оценка, а не измеренная вероятность.

## Возможности

- Пакеты до 1000 адресов, сохранение порядка, идентификаторов и точного `raw_address`.
- Выделение области, города, района, жилого массива, улицы, дома и деталей доставки.
- Native libpostal, правила явных маркеров, внутренний HTTP GLiNER и согласование свидетельств в Go.
- Неизменяемый справочник КАТО/OSM отдельно от каталога зданий.
- Подтверждение здания с проверкой ревизии и атомарным сохранением в JSON.
- Работа без интернета после подготовки образов и моделей; неизвестные поля остаются `null`.

## Быстрый запуск

Нужны Go 1.23+ и Python 3 для подготовки SQLite из включённого публичного снимка.

```sh
git clone https://github.com/lukivan8/jana-address.git
cd jana-address
make reference
REFERENCE_PATH="$PWD/data/reference/reference.sqlite" go run ./cmd/server
```

API доступен на `http://localhost:8080`. Такой запуск использует правила и справочник без нейромоделей; ответы явно содержат предупреждения о недоступности libpostal/GLiNER. Он подходит для разработки API и интеграции. Полный режим описан ниже.

```sh
curl -fsS http://localhost:8080/readyz
curl -sS http://localhost:8080/v1/normalize \
  -H 'Content-Type: application/json' \
  -d '{"records":[{"id":"order-1","raw_address":"г. Астана, проспект Туран 55/6 кв 99"}]}'
```

Ответ имеет форму `{"results":[...]}`. Поля `id` и `raw_address` совпадают с вводом; `coordinates` может быть `null`. Поддерживается также массив записей на верхнем уровне — ответом будет массив результатов. Полный формат и ошибки: [руководство API](docs/consumer-guide.md).

## Docker

Dockerfile собирается из корня этого репозитория и не читает соседние каталоги:

```sh
docker build -t jana-address:local .
docker volume create jana-address-catalog
docker run -d --name jana-address \
  -p 127.0.0.1:8080:8080 \
  -e GLINER_URL=http://YOUR-INTERNAL-GLINER:8090 \
  -v jana-address-catalog:/data \
  jana-address:local
```

`YOUR-INTERNAL-GLINER` замените адресом подготовленного GLiNER-сервиса, доступным из контейнера. Его `/readyz` должен отвечать успешно; протокол описан в [нормализации](docs/normalization.md). Python-код и веса не входят в этот Go-репозиторий. Готовая совместная оркестрация есть в [jana-locate](https://github.com/lukivan8/jana-locate).

При сборке скачиваются зависимости и модели libpostal; первый запуск сборки требует интернета, нескольких ГБ диска и времени. Во время запуска ничего не скачивается. Образ содержит libpostal и SQLite, работает от UID/GID `10001`, проверяет `/readyz` и сохраняет подтверждения в `/data`. В режиме `MODELS_REQUIRED=true` отсутствие libpostal или GLiNER останавливает запуск. Для намеренного запуска контейнера без GLiNER задайте `MODELS_REQUIRED=false`; libpostal останется доступен, а ответы отразят отсутствие второго прохода.

Для изолированной сети заранее соберите образ и перенесите его через `docker save` / `docker load`. Внешний GLiNER тоже должен иметь подготовленные локальные веса. Не публикуйте его порт наружу.

## HTTP API

| Метод и путь | Назначение |
| --- | --- |
| `GET /healthz` | Проверка работающего процесса |
| `GET /readyz` | Проверка готовности после загрузки каталога и обязательных зависимостей |
| `GET /openapi.json` | OpenAPI 3.0.3, те же байты, что в `api/openapi.json` |
| `POST /v1/normalize` | Пакетная нормализация |
| `GET /v1/catalog` | Каталог зданий с ревизиями |
| `POST /v1/catalog/confirmations` | Создание или подтверждение здания |

Встроенной авторизации нет. Для внешнего доступа настройте аутентификацию и TLS на шлюзе, особенно для подтверждений. CORS ограничивает браузерные источники и не заменяет авторизацию.

## Конфигурация

| Переменная | По умолчанию при `go run` | Назначение |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | Адрес HTTP; для контейнерного healthcheck используйте форму `:port` |
| `CATALOG_SEED_PATH` | `data/catalog.seed.json` | Начальный каталог |
| `CATALOG_OVERLAY_PATH` | `var/catalog.confirmed.json` | Файл подтверждений, каталог должен быть доступен на запись |
| `CORS_ORIGINS` | `http://localhost:3000` | Точные разрешённые origin через запятую |
| `REFERENCE_PATH` | не задана | Путь к неизменяемой SQLite; без него справочник не загружается |
| `LIBPOSTAL_DATA_DIR` | не задана | Локальные модели libpostal для native-сборки |
| `GLINER_URL` | не задана | Внутренний HTTP URL второго прохода |
| `MODELS_REQUIRED` | `false` | Значение `true` делает обе модели обязательными при запуске |
| `ALLOW_DEMO_CATALOG` | `false` | Значение `true` включает явно помеченные синтетические здания |

Docker задаёт `REFERENCE_PATH=/app/data/reference.sqlite`, `LIBPOSTAL_DATA_DIR=/models/libpostal`, `MODELS_REQUIRED=true`, seed `/app/data/catalog.seed.json` и overlay `/data/catalog.confirmed.json`. Остальные параметры можно переопределить через `-e`. Сервер завершает обработку при SIGTERM/SIGINT с бюджетом остановки 10 секунд.

## Go SDK

```sh
go get github.com/lukivan8/jana-address@latest
```

```go
import (
    "context"
    "github.com/lukivan8/jana-address/pkg/api"
    "github.com/lukivan8/jana-address/pkg/client"
)

sdk := client.New("http://localhost:8080")
result, err := sdk.Normalize(context.Background(), api.NormalizeRequest{
    Records: []api.InputRecord{{Id: "order-1", RawAddress: "Астана, Туран 55/6"}},
})
// Обработайте err, затем используйте result.Results.
```

`pkg/api` и `pkg/client` — публичные пакеты; SDK не импортирует `internal/` и использует стандартную HTTP-библиотеку. Таймаут SDK — 60 секунд. Для больших CPU-пакетов задайте `sdk.HTTP.Timeout = 300 * time.Second`. Ошибки API представлены как `*client.HTTPError` с `Status` и `Detail.Code`. [Компилируемый пример](examples/consumer/main.go):

```sh
JANA_API_URL=http://localhost:8080 go run ./examples/consumer
```

## Данные и хранение

Снимок содержит 15 586 записей КАТО и 1656 сущностей OSM: улицы и жилые территории в ограниченной области Астаны. Это не общенациональный справочник улиц. Справочник не содержит координат зданий. Источники, границы покрытия, хеши и атрибуция находятся в [manifest.json](data/reference/manifest.json); подготовка описана в [справочниках](docs/reference.md).

Здания хранятся отдельно: read-only seed и записываемый overlay. Ключ здания — город, тип улицы, название улицы и дом. Квартиры, подъезды, этажи и ориентиры не становятся значениями по умолчанию. Полные адресные строки и входные ID не используются как ключи кеша. Overlay имеет приоритет над seed и сохраняется при пересборке образа.

**Один файл overlay обслуживает один процесс.** Реплики не должны писать в общий том. Проверка ревизий защищает от конкурирующих запросов внутри процесса; изменение каталога атомарно заменяет файл. Гарантия не включает внезапное отключение питания. Резервное копирование, восстановление и изменение идентичности здания: [эксплуатация каталога](docs/catalog.md).

## Разработка и проверки

```sh
make check       # go test -race ./..., go vet ./..., go build ./...
make generate    # DTO из api/openapi.json, oapi-codegen v2.4.1
make reference   # неизменяемая SQLite из включённого JSONL
```

Обычные тесты и сборка не требуют моделей. Native-адаптер собирается с `CGO_ENABLED=1` и `-tags libpostal`, установленной C-библиотекой и подготовленными моделями. Dockerfile проверяет native-сборку. Сначала изменяйте `api/openapi.json`, затем генерируйте DTO; совместимые изменения сохраняйте в `/v1`. CI проверяет тесты, vet, сборку, справочник и отсутствие расхождений генерации.

```text
api/                  OpenAPI и встраивание спецификации
cmd/server/           запуск HTTP-сервера и healthcheck
pkg/api/              публичные сгенерированные DTO
pkg/client/           публичный HTTP SDK
internal/normalize/   разбор и согласование свидетельств
internal/postal/      native libpostal и fallback без CGO
internal/reference/   read-only SQLite
internal/catalog/     каталог, ревизии и атомарная запись
internal/httpapi/     маршруты, валидация, CORS и ошибки
data/                 публичный справочник и синтетический seed
tools/                подготовка SQLite
examples/consumer/    пример внешней интеграции
docs/                 API, хранение, модели и справочники
```

## Документация

- [Интеграция HTTP API](docs/consumer-guide.md)
- [Хранение, резервные копии и ревизии](docs/catalog.md)
- [Нормализация, модели и ограничения](docs/normalization.md)
- [Источники и сборка справочника](docs/reference.md)

Данные OSM: © OpenStreetMap contributors, [ODbL 1.0](https://www.openstreetmap.org/copyright). Атрибуция КАТО сохранена в записях и manifest. Условия сторонних библиотек, моделей и данных действуют отдельно.
