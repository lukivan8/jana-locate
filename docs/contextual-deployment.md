# Архив: подключение контекстного нормализатора

Этот эксперимент отключён. Действующий выпуск использует native + компактный CRF:
[подключение и откат](numeric-deployment.md). Ниже сохранена история contextual-выпуска;
его команды не являются текущей инструкцией деплоя.

Экспериментальный showcase запускался через `scripts/showcase.sh` с `compose.yaml`, `compose.contextual.yaml` и
`compose.showcase.yaml`. Native baseline остаётся обычным режимом самостоятельного backend без
`CONTEXTUAL_NORMALIZER_URL`. Оба режима используют прежний API, дополнительные поля описаны в OpenAPI.

Модель находится в отдельном read-only контейнере во внутренней сети. Runtime не скачивает assets.
Go API не загружает native модель, когда включён contextual сервис. Подготовка весов, контрольные
суммы и базовый образ описаны в [runtime README](../backend/contextual/README.md).

Перед переключением сохранены локальные Docker теги `jana-showcase-api:rollback-before-contextual`
и `jana-showcase-web:rollback-before-contextual`. Закрытая копия файлов хранилища и override для
отката находятся в `.deploy/backups/contextual-release/`, исключённом из Git. Работающее хранилище
не заменяется тестовыми данными; тесты используют отдельные volumes. Сохранённые задания не
пересчитываются автоматически — повторная нормализация требует нового задания.

## Откат

**Состояние после инцидента 18 октября 2026:** публичный API снова использует native
нормализатор. Сохранён совместимый новый API/интерфейс, но
`CONTEXTUAL_NORMALIZER_URL` пуст. Контекстный контейнер остановлен; watcher
`jana-showcase.service` остановлен и отключён от автозапуска. После этого конфигурация пересмотрена: `scripts/showcase.sh deploy` теперь использует
`compose.numeric.yaml` и не включает contextual-кандидата.

Старый API-образ не может читать журнал заданий с новыми полями результата
(`confidence_available` и другими): его strict JSON decoder останавливает запуск.
Поэтому откат через старый `rollback.yaml` после записи contextual-заданий **не подходит**.
Использовать совместимый образ API с отключённой моделью; текущий override
`.deploy/backups/contextual-release/native-compatible.yaml` фиксирует проверенные
образы по SHA. Журнал не преобразовывать и не удалять.

Сначала остановить watcher: `systemctl --user stop jana-showcase.service`.
Затем из корня выполнить:

```sh
docker compose -p jana-showcase --env-file .deploy/showcase.env \
  -f compose.yaml -f compose.contextual.yaml -f compose.showcase.yaml \
  -f .deploy/backups/contextual-release/native-compatible.yaml \
  up -d --no-build --wait backend frontend
```

Команда возвращает native режим, не удаляя рабочий volume. Убедиться, что `/readyz` отвечает,
сделать новый запрос нормализации и проверить интерфейс. Watcher оставлять остановленным, пока
исходники/настройка deploy не приведены к выбранному режиму; иначе он снова применит новый код.
Резервную копию данных автоматически не восстанавливать: это затрёт последующие правки оператора.

## Контроль выпуска

Проверяется действующий baseline и новый HTTP API на одних входах, отдельно исходный знакомый
корпус и контрольные наборы. Сравниваются не только совпадения, но и переходы correct→incorrect.
Выделения и справочные альтернативы упакованного кандидата сравниваются с лабораторным v7.
Для уверенности нет откалиброванной вероятности: `confidence_available=false`, UI «Не оценена».
Не считать `confidence=0` вероятностью ошибки; статус нового автоматического результата пока
`needs_review`. Операторское подтверждение сохраняет прежнее поведение.
