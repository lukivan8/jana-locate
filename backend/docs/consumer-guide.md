# Интеграция HTTP API

Базовый URL: `http://localhost:8080`. Контракт: [`api/openapi.json`](../api/openapi.json), также доступен по `GET /openapi.json`. Браузер должен использовать доступный ему URL, а не имя контейнера. Авторизация встроенно не реализована.

## Пакетная нормализация

```sh
curl -sS http://localhost:8080/v1/normalize \
  -H 'Content-Type: application/json' \
  -d '{"records":[{"id":"a","raw_address":"Туран 55/6 кв 99"}]}'
```

Объект `{"records":[...]}` получает ответ `{"results":[...]}`. Массив `[{"id":"a","raw_address":"..."}]` получает массив результатов. SDK использует объектную форму. Ошибки в обеих формах одинаковы.

Ограничения: 1–1000 записей, адрес до 4096 байт UTF-8, тело запроса до 2 MiB. Обязателен `Content-Type: application/json`. `id` — строка или int64, дубликаты разрешены. В JavaScript передавайте большие целые как строки. `raw_address` — обязательная строка; пустая строка допустима.

На каждую валидную запись возвращается один результат в исходном порядке. Точные ID и адрес сохраняются. Невалидный JSON, идентификатор или превышение лимита отклоняет весь пакет; неопределённость разбора не удаляет запись.

| Поля результата | Значение |
| --- | --- |
| `id`, `raw_address` | Исходные значения |
| `province`, `city`, `district`, `living_area` | Строка или явный `null` |
| `street_type`, `street_name`, `house` | Строка или `null`; дробь и литера дома сохраняются |
| `apartment`, `entrance`, `floor`, `landmark` | Детали конкретной доставки или `null` |
| `confidence` | Эвристическая оценка 0–1 |
| `coordinates` | WGS84: `{latitude, longitude}` либо `null` |
| `catalog_id`, `catalog_revision` | Здание и ревизия либо `null` |
| `status` | `matched`, `needs_review`, `unmatched` |
| `warnings` | Массив диагностических строк |

Геокодирование исключено из нормализации. `coordinates`, `catalog_id`, `catalog_revision` — устаревшие совместимые поля, всегда `null` у новых результатов normalize. `matched` означает полный разбор основных полей без неоднозначности, не поиск здания. Если остаются несколько совместимых справочных идентичностей населённого пункта, результат имеет `needs_review` даже при наличии точного адреса только для одной из них. Район по улице/дому дополняется после разрешения этой неоднозначности; явный выбор населённого пункта пересчитывает область и район. `confidence` и статус не зависят от каталога зданий и его координат. `libpostal_unavailable` означает резервный разбор. Новые результаты не содержат предупреждение `gliner_unavailable`; оно может встречаться в старых сохранённых заданиях.

Нормализация использует подготовленный справочник и не читает каталог зданий. Серверный бюджет ответа — 300 секунд, согласование моделей ограничено 270 секундами. У Go SDK таймаут по умолчанию 300 секунд; задайте context, покрывающий время обработки пакета.

## Legacy API каталога зданий

Эти endpoints сохранены для совместимости. Они не участвуют в нормализации и не улучшают её результаты.

### Подтверждение здания

`GET /v1/catalog` возвращает `{"entries":[...]}`. Каждая запись содержит `id`, `revision`, `fields`, `coordinates`, `source`, `confirmed`.

`POST /v1/catalog/confirmations` принимает полную структуру полей, проверенные координаты и `expected_revision`: 0 для нового здания, текущую ревизию для существующего. Успешный ответ — сохранённая запись с увеличенной ревизией, `confirmed: true` и `source: "manual-confirmation"`.

```json
{
  "fields": {
    "province": "Астана", "city": "Астана", "district": null,
    "living_area": null, "street_type": "проспект", "street_name": "Туран",
    "house": "55/6", "apartment": null, "entrance": null,
    "floor": null, "landmark": null
  },
  "coordinates": {"latitude": 51.128, "longitude": 71.43},
  "expected_revision": 0
}
```

**Координаты примера синтетические.** Не используйте их как проверенный адрес.

`city`, `street_type`, `street_name`, `house` обязательны и непустые. Детали доставки должны быть `null`. Широта: −90…90, долгота: −180…180. Пропущенные nullable-ключи сейчас трактуются как `null`, но клиентам рекомендуется отправлять полную форму контракта.

При HTTP 409 `revision_conflict` перечитайте запись и согласуйте изменения; автоматическая подстановка новой ревизии может затереть чужую правку. Если ответ потерян после записи, повторный POST может получить 409: сравните сохранённое состояние с намерением пользователя.

Ключ — `(city, street_type, street_name, house)`. Изменение ключевого поля создаёт другое здание. Удаление, слияние и переименование через API не реализованы.

## Ошибки

Ошибки обработчиков имеют вид `{"code":"...","message":"..."}`.

| HTTP | Причина |
| --- | --- |
| 400 | Невалидные поля или запрос |
| 403 | Запрещённый Origin |
| 409 | Конфликт ревизии |
| 413 | Превышение размера тела |
| 415 | Неподдерживаемый Content-Type |
| 500 | Ошибка хранения |

Неизвестные пути и методы используют стандартные ответы Go 404/405: тело не обязательно JSON. В Go проверяйте `*client.HTTPError`, его `Status` и `Detail.Code`.

CORS разрешает точные значения `CORS_ORIGINS`. Серверные запросы без Origin работают независимо от этого списка. CORS не является механизмом авторизации.

Для других языков генерируйте клиента из OpenAPI. Фронтенд может хранить собственную копию контракта и обновлять её отдельно; исходники Go ему не нужны.

## Сохраняемая пакетная проверка

Для продолжения работы после перезапуска используйте отдельный [API заданий](jobs.md). Синхронный `/v1/normalize` не меняется.

## Выбор населённого пункта в собственном журнале

`POST /v1/normalize/candidates` принимает `{record: {id, raw_address}}` и возвращает `{reference_version, candidates}`. `POST /v1/normalize/selections` принимает `{record, reference_version, entity_id}` и пересчитывает исходную запись тем же движком, что используется заданиями сайта. Несовместимая сущность отклоняется с 400, устаревший справочник — с 409. Эти операции не создают задание и не сохраняют ответы; ревизиями собственного журнала управляет потребитель. Выбор населённого пункта не подтверждает все поля адреса и не создаёт координаты или запись в каталоге зданий.

## Контекстный нормализатор v7

Hosted API может возвращать `normalizer_version=contextual-v7-grounded-microalpha-lean-v1`.
При `confidence_available=false` поле `confidence=0` — только совместимый числовой placeholder;
показывайте «Не оценена». До калибровки автоматические ответы имеют `needs_review`.
Дополнительное поле `normalization_details` содержит исходные выделения, альтернативы,
справочные кандидаты и диагностику. Офсеты spans считаются в Unicode code points.
Сбой/занятость inference возвращает HTTP 503 `normalizer_unavailable`; повторите запрос позже.
Сохраняемое задание при таком сбое получает состояние failed. Старые сохранённые результаты
не пересчитываются автоматически. Для повторного разбора создайте новое задание.

Расширенная диагностика может увеличивать ответ на 1000 записей сверх 16 МиБ. Текущие исходники Go SDK и CLI принимают до 128 МиБ; старым клиентам с жёстким лимитом требуется обновление либо меньший пакет. Таймаут Go SDK по умолчанию — 300 секунд.

## Experimental native numeric supplement

`NUMERIC_SUPPLEMENT_URL` optionally enables an offline numeric-only CRF service
on top of native normalization. It is mutually exclusive with
`CONTEXTUAL_NORMALIZER_URL`. Default behavior is unchanged. Missing proposals,
transport failure or invalid source spans fall back to the original native result.
The supplement can only fill null `house`, `apartment`, `entrance`, `floor`; existing
values, geographic candidates, status and confidence are not replaced. Records
with ambiguity/conflict warnings are left unchanged. Duplicate role proposals,
low token scores and overlaps with existing textual components are rejected.

Added fields carry `normalizer_version=native-numeric-crf-v1` and provenance in
`normalization_details.numeric_supplement`. The existing confidence still describes
native evidence; the tagger posterior is not a calibrated probability for the
address. `confidence_scope` records this distinction. No automatic promotion from
`needs_review` to `matched` is introduced. Runtime never downloads model assets.
This mode remains a candidate until non-regression and transfer checks are accepted.
