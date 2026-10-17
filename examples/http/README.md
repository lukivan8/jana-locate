# Примеры HTTP-интеграции

[Руководство подключения](../../docs/integration.md) · [OpenAPI](../../backend/api/openapi.json)

Сначала [запустите API](../../docs/quickstart.md). Команды выполняются из корня репозитория. Дополнительные пакеты не требуются.

| Пример | Требование | Команда |
| --- | --- | --- |
| cURL | curl с `--fail-with-body` | `curl --fail-with-body http://localhost:8080/v1/normalize -H 'Content-Type: application/json' --data-binary @examples/http/request.json` |
| [JavaScript](normalize.mjs) | Node.js 22.12+ | `node examples/http/normalize.mjs` |
| [Python](normalize.py) | Python 3 | `python3 examples/http/normalize.py` |
| [Go](go/main.go) | Go 1.23+ | `go run examples/http/go/main.go` |

Переменная `JANA_API_URL` меняет endpoint у примеров JavaScript/Python/Go. Во всех случаях отправляется одинаковый синтетический адрес. Примеры обрабатывают HTTP/сетевую ошибку, проверяют количество записей и сохранение ID/исходной строки. Они печатают весь JSON, включая статус и предупреждения; бизнес-решение о принятии адреса остаётся за вашим приложением.

[response.fallback.json](response.fallback.json) — сохранённый ответ актуальной версии в режиме без native libpostal с включённым справочником. Он показывает формат и явное предупреждение о резервном режиме. Полный native-режим может дать другие поля/confidence. Этот файл не является эталоном точности.
