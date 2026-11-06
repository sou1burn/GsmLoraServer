# GsmLoraServer

TCP-сервер между клиентским [интерфейсом](https://github.com/Ykio7614/LoRaStandInterface) и мастер-устройством LoRa (ESP32).
Принимает команды от интерфейса, передаёт их на ESP, усредняет пришедшие измерения и сохраняет их в PostgreSQL по сессиям.

```
Интерфейс ──TCP──▶ GsmLoraServer ──TCP──▶ ESP32 (master) ~~LoRa~~ slave
                         │
                     PostgreSQL
```

## Запуск

Нужны Docker и Docker Compose.

```
git clone https://github.com/sou1burn/GsmLoraServer.git
cd GsmLoraServer
docker compose up -d --build
```

Приложение работает в `network_mode: host` и слушает порт из `PORT` (по умолчанию `9090`).
Postgres публикуется на `127.0.0.1:5433`. Схема БД создаётся миграциями при старте, руками ничего готовить не нужно.

Сборка с race-детектором: `docker compose -f docker-compoce.race.yaml up -d --build`.

Полностью стенд (сервер + интерфейс) поднимают скрипты `install_everything.sh` и `run_all.sh`.

### Локально без Docker

```
go run .
```

Нужен Go 1.25 и доступный Postgres из `db.env`.

## Конфигурация

Читается из env-файлов в рабочей директории (переменные окружения имеют приоритет):

| Файл | Переменные |
|------|------------|
| `db.env` | `PORT`, `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `ENV`, а также `POSTGRES_*` для контейнера БД |
| `esp.env` | `ESP_HOST`, `ESP_PORT`, `ESP_MAC`, `ESP_IP` |
| `client.env` | `CLIENT_IP`, `CLIENT_PORT` |

`make_envs.sh` создаёт эти файлы со значениями по умолчанию.

## Протокол

Текстовый, одно сообщение на строку (`\n`).

**Интерфейс → сервер**

| Команда | Действие |
|---------|----------|
| `SET_SETTINGS: SF=<f>, TX=<f>, BW=<f>` | передать параметры LoRa на ESP |
| `START_MEASUREMENT: <session_id>` | начать измерения в сессии |
| `GET_MEASUREMENT_SESSIONS` | список сессий + текущие статусы устройств |
| `ADD_SESSION: [id, name, start, end, count]` | создать сессию |
| `REMOVE_SESSION: <id>` | удалить сессию с измерениями |

Если ESP не подключён, `SET_SETTINGS` и `START_MEASUREMENT` откладываются и отправляются после подключения (сначала настройки, потом старт).

**ESP → сервер**

| Сообщение | Значение |
|-----------|----------|
| `IDENTIFY` | ESP представился, соединение запоминается как мастер |
| `MEASUREMENT: {json}` | пакет измерения (`rssi`, `snr`, `hdop`, `coordinate`, опционально `timestamp` в RFC 3339) |
| `ACK: SSET SF<f>,BW<f>,TX<f>` | ESP применил настройки |
| `ACK` | heartbeat |
| `MASTER_STATUS <s>` / `SLAVE_STATUS <s>` | статус устройств |

**Сервер → интерфейс**

`SESSIONS: [...]`, `MEASUREMENT: [...]`, `MASTER_STATUS CONNECTED|DISCONNECTED`, `SLAVE_STATUS ...`, `SETTINGS_ACK: ...`, `ALREADY_SET`, `ERROR: <текст>`.

**Сервер → ESP**: `SET_SETTINGS: SF=..., TX=..., BW=...`, `START_MEASUREMENT SESSION_ID=<id>`.

Пакеты буферизуются и усредняются; если 15 с нет новых пакетов, буфер сбрасывается. Если от мастера 120 с ничего не приходит, интерфейсу отправляется `MASTER_STATUS DISCONNECTED`.

## Структура

```
main.go              точка входа
cmd/app              сборка зависимостей, подключение к БД
cmd/server           TCP-сервер, разбор входящих команд, статусы устройств
internal/handlers    парсинг сообщений протокола
internal/service     обработка пакетов, отложенные команды, рассылка клиентам
internal/esp         соединение с ESP32
internal/postgres    репозиторий и запуск миграций
db/migrations        SQL-миграции goose (встроены в бинарник)
test/unit            тесты
doc                  сценарии и диаграммы
```

## База данных

Миграции goose применяются автоматически при старте (`postgres.Migrate`). Новая миграция — файл `db/migrations/NNNNN_name.sql` с секциями `-- +goose Up` / `-- +goose Down`.

Полезные команды:

```
docker compose logs -f app                                          # логи сервера
docker compose exec postgres psql -U postgres -d lora_db            # консоль psql
docker compose exec postgres psql -U postgres -d lora_db -c "QUERY" # один запрос
```

## Тесты

```
go test ./...
go test -race ./...   # нужен cgo (gcc)
```

## Документация

- [Сценарии взаимодействия с ESP](doc/Esp_scenarios.pdf)
- Диаграммы PlantUML: `doc/*.puml`
