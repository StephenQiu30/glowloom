SHELL := /bin/bash
.DEFAULT_GOAL := help

ENV_FILE := $(if $(wildcard docker-compose-env.env),docker-compose-env.env,docker-compose-env.example)
COMPOSE := docker compose --env-file $(ENV_FILE)
MIDDLEWARE := postgres redis kafka minio temporal temporal-ui kafka-ui
GOBIN := $(shell go env GOPATH)/bin

.PHONY: help up down ps logs app-up app-down check check-backend check-agent check-frontend

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-16s %s\n", $$1, $$2}'

up: ## Start local middleware (PostgreSQL, Redis, Kafka, MinIO, Temporal, UIs)
	$(COMPOSE) up -d --wait $(MIDDLEWARE)

down: ## Stop everything (volumes are kept)
	$(COMPOSE) down

ps: ## Show container status
	$(COMPOSE) ps

logs: ## Follow logs (make logs s=temporal)
	$(COMPOSE) logs -f $(s)

app-up: ## Build and start frontend, backend-api and agent-api containers with middleware
	$(COMPOSE) up -d --build --wait

app-down: ## Stop the application containers only
	$(COMPOSE) stop frontend backend-api agent-api

check: check-backend check-agent check-frontend ## Run every quality gate

check-backend: ## gofmt, goimports, vet, golangci-lint, race tests, govulncheck
	cd backend && test -z "$$(gofmt -l .)" \
	  && test -z "$$($(GOBIN)/goimports -local github.com/StephenQiu30/lanverse/backend -l .)" \
	  && go vet ./... && $(GOBIN)/golangci-lint run ./... \
	  && go test -race ./... && $(GOBIN)/govulncheck ./...

check-agent: ## ruff, ruff format, mypy, pytest
	cd agent && uv sync --locked && uv run ruff check . && uv run ruff format --check . \
	  && uv run mypy app tests && uv run pytest

check-frontend: ## eslint, prettier, typecheck, vitest, build
	cd frontend && pnpm install --frozen-lockfile && pnpm lint && pnpm format:check \
	  && pnpm typecheck && pnpm test && pnpm build
