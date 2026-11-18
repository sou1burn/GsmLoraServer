# ============================================================================
# GsmLoraServer — Makefile
# opencode -s ses_e1937e03cffeNx6BB46RIAEtCC
# Цели согласованы с планом эволюции (GsmLoraServer_evolution_plan.md):
#   этап 0: lint → test (race) → build/up — база для CI-пайплайна;
#   этап 1+: новые сервисы (cmd/ingestion, cmd/api, cmd/storage) просто
#            дописываются в переменную BINARIES ниже — остальные цели
#            не меняются.
#
# Окружение: Linux / WSL / Git Bash (нужны make, Go >= 1.25, docker compose).
# ============================================================================

BINARY_NAME ?= gsmloraserver

# Формат «имя:путь к main-пакету». Сейчас монолит в корне; после этапа 1
# плана список станет, например:
#   BINARIES := ingestion:./cmd/ingestion api:./cmd/api storage:./cmd/storage
# Цели build/test/lint/docker при этом менять не нужно.
BINARIES := $(BINARY_NAME):.

# Версия — из git-тегов, иначе dev. Прокидывается в бинарник (main.version)
# и в docker-образ (ARG VERSION в Dockerfile).
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)

GO ?= go
GOLANGCI_LINT_VERSION ?= v2.15.0   # менять синхронно с CI-пайплайном
MOCKGEN_VERSION ?= v1.6.0          # должен совпадать с gomock из go.mod
DOCKER_COMPOSE ?= docker compose   # или docker-compose, если плагина нет

.DEFAULT_GOAL := help
.PHONY: help build build-cross run test test-race race lint lint-fix fmt vet \
        tidy mocks tools envs up down logs ps docker-build clean ci

# ---------------------------------------------------------------- Разработка

help: ## Показать список целей
	@echo "$(BINARY_NAME) v$(VERSION) — цели (детали: GsmLoraServer_evolution_plan.md)"
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

build: ## Собрать бинарник(и) из BINARIES в bin/
	@ext=""; \
	if [ "$$($(GO) env GOOS)" = "windows" ]; then ext=".exe"; fi; \
	for entry in $(BINARIES); do \
		name=$${entry%%:*}; pkg=$${entry#*:}; \
		echo "→ build $$pkg → bin/$$name$$ext"; \
		CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' \
			-o bin/$$name$$ext $$pkg || exit 1; \
	done

build-cross: ## Кросс-сборка монолита: linux/darwin/windows (amd64)
	@for osarch in linux/amd64 darwin/amd64 windows/amd64; do \
		os=$${osarch%/*}; arch=$${osarch#*/}; ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "→ build $(BINARY_NAME)-$$os-$$arch$$ext"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath \
			-ldflags '$(LDFLAGS)' -o bin/$(BINARY_NAME)-$$os-$$arch$$ext . || exit 1; \
	done

run: ## Запустить монолит локально: go run . (нужны db.env и Postgres)
	$(GO) run .

test: ## Юнит-тесты
	$(GO) test ./...

test-race: ## Юнит-тесты под race-детектором (как в CI)
	$(GO) test -race -count=1 ./...

race: ## Поднять стенд с race-детектором (docker-compose.race.yaml)
	@$(DOCKER_COMPOSE) down --remove-orphans >/dev/null 2>&1 || true
	@test -f db.env || $(MAKE) envs
	$(DOCKER_COMPOSE) -f docker-compose.race.yaml up -d --build
	@echo "Логи: make logs   (контейнер gsmloraserver_app)"

# ---------------------------------------------------------------- Код

lint: ## Линтинг всего репозитория (.golangci.yml, golangci-lint v2)
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint не найден — установите: make tools"; exit 1; }
	golangci-lint run

lint-fix: ## Автоисправление замечаний линтеров (golangci-lint run --fix)
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint не найден — установите: make tools"; exit 1; }
	golangci-lint run --fix

fmt: ## Форматирование gofmt + goimports (golangci-lint fmt)
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint не найден — установите: make tools"; exit 1; }
	golangci-lint fmt

vet: ## go vet ./...
	$(GO) vet ./...

tidy: ## go mod tidy + проверка целостности зависимостей
	$(GO) mod tidy
	$(GO) mod verify

mocks: ## Регенерация gomock-моков в mocks/
	@command -v mockgen >/dev/null 2>&1 || { \
		echo "mockgen не найден — установите: make tools"; exit 1; }
	mockgen -source internal/core/interfaces.go -destination mocks/mock_core.go -package=core
	mockgen -source internal/esp/interface.go -destination mocks/esp_mock.go -package=core
	mockgen -source internal/service/service.go -destination mocks/mock_service.go -package=core

tools: ## Установить dev-инструменты: golangci-lint, mockgen
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	$(GO) install github.com/golang/mock/mockgen@$(MOCKGEN_VERSION)
	@echo "Проверьте, что $$($(GO) env GOPATH)/bin есть в PATH"

# ---------------------------------------------------------------- Стенд (Docker)

envs: ## Сгенерировать db.env / esp.env / client.env (make_envs.sh)
	bash make_envs.sh

up: ## Поднять стенд: postgres + приложение
	@test -f db.env || $(MAKE) envs
	$(DOCKER_COMPOSE) up -d --build

down: ## Остановить стенд (том Postgres сохраняется)
	$(DOCKER_COMPOSE) down --remove-orphans

logs: ## Логи приложения (docker compose logs -f app)
	$(DOCKER_COMPOSE) logs -f app

ps: ## Статус контейнеров стенда
	$(DOCKER_COMPOSE) ps

docker-build: ## Собрать образ приложения с версией из git
	@test -f db.env || $(MAKE) envs
	docker build --build-arg VERSION=$(VERSION) --build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(BINARY_NAME):$(VERSION) .

# ---------------------------------------------------------------- Прочее

clean: ## Удалить bin/ и кэш тестов
	rm -rf bin
	$(GO) clean -testcache

ci: lint test-race build ## Локальный CI-прогон: lint → race-тесты → сборка
