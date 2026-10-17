# Пример базы заказов

Самостоятельная PostgreSQL-база `orders_example` с таблицей `public.orders` и 2000 вымышленными заказами — по одному на каждый `records[]` из `address-anomalies.json`. Это демонстрационные данные, без эталонной нормализации и реальных клиентов.

| Колонка | Содержимое |
| --- | --- |
| `id` | Числовой первичный ключ; новые строки получают ID автоматически |
| `anomaly_id` | Исходный ID из набора, например `synthetic_001` |
| `name` | Случайное вымышленное имя |
| `order_date` | Случайная дата от 2025-01-01 до 2026-10-15 |
| `address` | Случайный адрес клиента из того же набора |
| `delivery_address` | Точный `raw_address` соответствующего примера, включая пробелы/переносы/опечатки |

Для CLI выбирайте колонку **delivery_address**. Имена и даты генерируются с фиксированным seed, поэтому SQL воспроизводим. Адреса — непроверенные синтетические варианты; публичные исходные компоненты © OpenStreetMap contributors, [ODbL-1.0](https://www.openstreetmap.org/copyright). Происхождение и категории остаются в исходном JSON; `anomaly_id` связывает с ними заказы.

Из корня репозитория:

```sh
docker compose -f cli/examples/orders/compose.yaml up -d --wait
```

Подключение с хоста:

```text
host:     localhost
port:     15432
database: orders_example
user:     orders_demo
password: example-only-password
schema:   public
table:    orders
key:      id
column:   delivery_address
```

Пароль публичный, только для примера; порт доступен лишь на loopback. Другой порт задаётся через `ORDERS_EXAMPLE_PORT`. Для CLI через `./cli/jana` на обычном Linux Engine используйте те же `localhost:15432`: CLI работает в сети хоста. На Docker Desktop в обычном bridge-режиме используйте `host.docker.internal:15432`. Для других контейнеров в bridge-сети доступ к этой базе зависит от их Docker-сетей; этот отдельный пример публикует порт только на loopback хоста.

Пример с локальным бинарником CLI из корня репозитория (сборка: `cd cli && go build -o bin/jana ./cmd/jana`); API нормализации должен работать отдельно:

```sh
JANA_DATABASE_URL='postgresql://orders_demo:example-only-password@localhost:15432/orders_example?sslmode=disable' \
  ./cli/bin/jana connect --state cli/var/orders-example.sqlite \
  --table orders --column delivery_address --target-table orders_normalized \
  --api-url http://localhost:8080 --no-interactive
./cli/bin/jana normalize --state cli/var/orders-example.sqlite
./cli/bin/jana publish --state cli/var/orders-example.sqlite
```

Параметры можно ввести и через интерактивный `connect`. Каждый адрес доставки попадёт в локальный журнал; после `confirm` в `publish` появится отдельная связанная таблица `orders_normalized`.

Просмотр заказов:

```sh
docker compose -f cli/examples/orders/compose.yaml exec postgres \
  psql -U orders_demo -d orders_example \
  -c 'SELECT id, name, order_date, delivery_address FROM orders ORDER BY id LIMIT 10;'
```

Вместо Docker можно импортировать `seed.sql` через `psql --set ON_ERROR_STOP=1 --dbname YOUR_EMPTY_DEMO_DATABASE --file cli/examples/orders/seed.sql`. Скрипт создаёт `public.orders`, не удаляет существующие таблицы и завершается ошибкой, если она уже существует.

Регенерация из актуального набора (явная операция подготовки, без зависимости от frontend при запуске):

```sh
python3 cli/examples/orders/generate.py --source frontend/src/data/address-anomalies.json
```

Compose использует готовый SQL и собственный том. Повторный запуск сохраняет базу; обновлённый seed применяется только к новой пустой базе. Остановка: `docker compose -f cli/examples/orders/compose.yaml down`. Удаление демонстрационной базы вместе с томом: та же команда с `--volumes`.
