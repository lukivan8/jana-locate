# Jana Locate

Сервис нормализации адресов Казахстана с веб-интерфейсом проверки результатов. Выделяет компоненты адреса, сопоставляет здания с каталогом, показывает неопределённость и позволяет подтверждать проверенные данные.

Полный комплект: независимый Go API, TanStack Start SPA и внутренний CPU GLiNER. После подготовки Docker-образов работает без интернета. Самостоятельный Go backend и SDK опубликованы отдельно: [jana-address](https://github.com/lukivan8/jana-address).

> Здания и координаты начального каталога синтетические и отключены по умолчанию. Координаты неизвестных или неоднозначных зданий возвращаются как `null`. Оценка уверенности эвристическая; измеренная точность на реальных адресах не заявляется.

## Запуск

Нужны Docker Engine и Compose v2:

```sh
git clone https://github.com/lukivan8/jana-locate.git
cd jana-locate
cp .env.example .env
docker compose up --build --wait
```

- Интерфейс: `http://localhost:3000`
- Готовность API: `http://localhost:8080/readyz`
- OpenAPI: `http://localhost:8080/openapi.json`

Первая сборка требует интернета, нескольких ГБ диска и памяти для моделей; веса и зависимости скачиваются при сборке. Runtime ничего не скачивает. Для закрытой сети заранее перенесите все три образа через `docker save` / `docker load`. GLiNER не публикует порт на хост.

## Работа с адресами

В интерфейсе вставьте адреса или загрузите JSON, выполните нормализацию, проверьте неизвестные поля и предупреждения, при необходимости отредактируйте данные и экспортируйте результат. Подтверждение сохраняет только структуру здания и проверенные координаты. Квартира, подъезд, этаж и ориентир относятся к текущей доставке.

Для знакомства включите `ALLOW_DEMO_CATALOG=true` в `.env` и пересоздайте контейнеры. Демокоординаты сохраняют предупреждение о синтетическом происхождении. Встроенный набор сложных примеров тоже синтетический, без эталонных ответов.

```sh
curl -sS http://localhost:8080/v1/normalize \
  -H 'Content-Type: application/json' \
  -d '{"records":[{"id":"order-1","raw_address":"Астана, Туран 55/6 кв 99"}]}'
```

Принимаются объект `{"records":[...]}` или массив записей; ответы соответственно `{"results":[...]}` или массив. Сервис сохраняет исходный порядок, ID и строки, возвращает явные `null` для неизвестных полей. [Описание API](backend/docs/consumer-guide.md).

## Архитектура

```text
frontend/       самостоятельная SPA и Nginx, только HTTP к внешнему API
backend/        самостоятельный Go-модуль, Dockerfile, API и SDK
nlp/            внутренний CPU GLiNER, веса готовятся при сборке
scripts/        проверка контрактов, справочников и Docker-персистентности
```

Go согласует libpostal, правила, GLiNER, КАТО/OSM и каталог зданий. Фронтенд собирается без Go-исходников и работающего API. Go собирается без frontend и Python-исходников; GLiNER подключается по HTTP во время работы. `backend/api/openapi.json` — основной контракт; фронтенд хранит отдельную копию и сгенерированные TypeScript-типы.

КАТО/OSM — неизменяемая SQLite без координат зданий. Публичный снимок: 15 586 записей КАТО и 1656 сущностей OSM в ограниченной области Астаны. Подтверждения — отдельный JSON overlay в томе `catalog-data`. Полные адреса и входные ID не становятся ключами кеша.

## Настройки и данные

`.env.example` описывает порты, браузерный URL API, CORS и параметры GLiNER. `JANA_API_URL` должен быть доступен браузеру; имя контейнера `backend` для этого не подходит. CORS должен содержать точный origin интерфейса. Порты по умолчанию доступны только на loopback. Для внешнего доступа настройте TLS и авторизацию на шлюзе: встроенной авторизации у API нет.

`docker compose down` сохраняет подтверждения; `docker compose down --volumes` удаляет их. Один API-процесс должен владеть одним overlay. Не направляйте тесты на рабочий каталог. [Хранение и восстановление](backend/docs/catalog.md).

## Разработка

Нужны Go 1.23+, Node 22.12+ и Python 3 для справочника:

```sh
make reference
cd backend
REFERENCE_PATH="$PWD/data/reference/reference.sqlite" go run ./cmd/server
```

В другом терминале:

```sh
cd frontend
npm ci
VITE_JANA_API_URL=http://localhost:8080 npm run dev
```

Без native-тега libpostal и отдельного GLiNER это ограниченный режим с явными предупреждениями. Полный режим предоставляется Docker Compose. Подробнее: [backend](backend/README.md), [frontend](frontend/README.md), [GLiNER](nlp/README.md).

Go backend в `backend/` — включённая копия самостоятельного [jana-address](https://github.com/lukivan8/jana-address), а не submodule. Изменения между репозиториями синхронизируются явно; публикация SDK относится к корневому модулю `jana-address`.

## Проверки

```sh
make check          # race-тесты, vet, frontend build/typecheck, равенство контрактов
make generate       # генерация Go DTO и TypeScript-типа из OpenAPI
make reference      # SQLite из публичного снимка
make smoke          # временный Compose-проект: HTTP, SPA и сохранение после рестарта
make offline-smoke  # запуск подготовленных образов без внешней сети
```

Браузерные тесты выполняются против API с отдельным временным overlay:

```sh
cd frontend
npx playwright install chromium
npm run test:e2e
```

CI проверяет приложение, генерацию, браузерный сценарий и Docker. Smoke создаёт отдельные тома и удаляет их после проверки. Данные OSM: © OpenStreetMap contributors, [ODbL 1.0](https://www.openstreetmap.org/copyright); источники и хеши находятся в [manifest](backend/data/reference/manifest.json).
