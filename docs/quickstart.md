# Быстрый запуск

[Документация](README.md) · [Интеграция](integration.md)

## 1. Подготовьте окружение

Установите Git, Docker Engine и Docker Compose v2. Для первой сборки нужен интернет: скачиваются зависимости и модели libpostal. Предусмотрите несколько ГБ свободного места и память для сборки. В Compose API ограничен 3 GiB RAM и двумя CPU; это настройка контейнера, не измеренные минимальные требования.

```sh
git clone https://github.com/lukivan8/jana-locate.git
cd jana-locate
cp .env.example .env
```

Готовые публичные контейнерные образы пока не выпускаются. Все Docker-команды в этом руководстве собирают исходники локально.

## 2. Выберите способ работы

### Только HTTP API

```sh
docker compose up --build --wait backend
```

Это запускает нормализатор без веб-интерфейса. Не требуется подключение PostgreSQL или отдельный Python-сервис. Native libpostal и справочник подготавливаются внутри образа.

### API и веб-интерфейс

```sh
docker compose up --build --wait
```

Откройте `http://localhost:3000`. API работает на `http://localhost:8080`.

### CLI с локальным журналом

```sh
./cli/jana up
./cli/jana connect
./cli/jana normalize
```

Сайт CLI доступен на `http://localhost:8095`. В переносимом стенде по умолчанию используются пользователь `jana` и демонстрационный пароль `local-example-password`; перед использованием задайте собственный `JANA_WEB_TOKEN`. PostgreSQL не запускается автоматически. Для файлов и всех флагов см. [CLI](../cli/README.md).

Не запускайте одновременно стенды на одинаковом порту API. Другие порты задаются через `.env` или переменные окружения; см. [развёртывание](deployment.md).

## 3. Проверьте API

```sh
curl --fail-with-body http://localhost:8080/readyz
curl --fail-with-body http://localhost:8080/v1/normalize \
  -H 'Content-Type: application/json' \
  --data-binary @examples/http/request.json
```

Успешный HTTP 200 означает обработку запроса. Проверьте `status`, `warnings` и неизвестные поля каждой записи. HTTP 200 не означает, что все адреса однозначны.

[Следующий шаг: подключение своего сервиса](integration.md).

## Запуск из исходников без Docker

Для разработки нужны Go 1.23+, Python 3 и, для интерфейса, Node.js 22.12+. Python только готовит SQLite; в runtime он не нужен.

Из корня репозитория:

```sh
make reference
cd backend
REFERENCE_PATH="$PWD/data/reference/reference.sqlite" go run ./cmd/server
```

Без native-тега libpostal работает резервный разбор с `libpostal_unavailable`. Этот режим проверяет интеграцию, но не воспроизводит качество полного Docker-режима.

В другом терминале из корня:

```sh
cd frontend
npm ci
VITE_JANA_API_URL=http://localhost:8080 npm run dev
```

Для native-сборки см. [режимы нормализации](../backend/docs/normalization.md).

## Остановка

```sh
docker compose down
```

Команда сохраняет том с заданиями и данными приложения. `docker compose down --volumes` удаляет данные. Остановка CLI-стенда описана отдельно в его руководстве.

## Если запуск не удался

```sh
docker compose ps
docker compose logs --tail=100 backend
```

Если порт занят, измените `API_PORT`/`WEB_PORT` и согласуйте `JANA_API_URL`/`CORS_ORIGINS`. Если готовность не наступает, проверьте логи, память и наличие подготовленных моделей. [Частые вопросы](faq.md).
