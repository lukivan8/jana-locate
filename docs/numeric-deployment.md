# Публичный выпуск native + numeric CRF, 18 октября 2026

На jana.linker.kz подключён `native-numeric-crf-v1`. Native libpostal, справочники,
статусы, неоднозначности и confidence сохранены. Отдельный компактный CRF дополняет
только пустые числовые поля по проверенной политике. При его отказе API возвращает baseline.
Процент отражает прежнюю native оценку, а не откалиброванную вероятность всей записи.

`scripts/showcase.sh` использует `compose.yaml`, `compose.numeric.yaml`,
`compose.showcase.yaml`. `CONTEXTUAL_NORMALIZER_URL` пуст; числовой сервис доступен
только внутри runtime-сети. Подготовленные веса входят в образ, скачиваний при запуске нет.
Старый contextual-контейнер остановлен. Watcher `jana-showcase.service` отключён.

Публичный API проверен на 4608 строках: 0 смысловых различий с проверенным кандидатом
по полям, статусам, confidence (допуск 1e-12), предупреждениям и версии.
Пакеты по 1000 заняли 1.66–3.15 секунды, последние 608 — 1.50 секунды.
Первый прогон прервался из-за TLS ошибки curl 35; повторный полный прогон завершён.
Проверены UI desktop и 390×844: проценты 82%/94%, квартира/подъезд/этаж,
без ошибок console. Проверка не подтверждала синтетические здания в каталог.
Подробное сравнение качества: [отчёт](numeric-recovery-results.md).

## Откат

Перед выпуском сохранены совместимые образы `jana-showcase-api:rollback-before-numeric`
и `jana-showcase-web:rollback-before-numeric`. Закрытая резервная копия хранилища:
`.deploy/backups/numeric-release/data-before.tar`. Не восстанавливать её автоматически:
это затрёт новые действия операторов. Старые задания не пересчитываются — отправить пакет заново.

```sh
docker compose -p jana-showcase --env-file .deploy/showcase.env \
  -f compose.yaml -f compose.numeric.yaml -f compose.showcase.yaml \
  -f .deploy/backups/numeric-release/rollback.yaml \
  up -d --no-build --wait backend
```

Override отключает оба дополнительных нормализатора и возвращает совместимый native API.
Проверить `/readyz` и новый запрос. Не использовать древний API до contextual-выпуска:
он не читает новые поля журнала. Рабочий volume при переключении не заменялся.
