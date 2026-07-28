SHELL := /bin/sh

.PHONY: setup dev-api dev-frontend db-init generate-api lint typecheck test hygiene check docker-build compose-up compose-down identity-up identity-down contract contract-oidc e2e-install e2e

setup:
	@uv --version | grep -q '0.11.32'
	@node -v | grep -q 'v22.23.1'
	@npm -v | grep -q '10.9.8'
	uv sync --project backend --frozen --no-python-downloads
	cd frontend && npm ci

dev-api:
	cd backend && uv run --frozen --no-python-downloads uvicorn app.main:app --reload --host 127.0.0.1 --port 8000

dev-frontend:
	cd frontend && npm run dev

db-init:
	cd backend && uv run --frozen --no-python-downloads python -m app.initialize_database

generate-api:
	@test -n "$$OPENAPI_SCHEMA_URL" || (echo 'OPENAPI_SCHEMA_URL is required' >&2; exit 1)
	cd frontend && OPENAPI_SCHEMA_URL="$$OPENAPI_SCHEMA_URL" npm run openapi2ts

lint:
	cd backend && uv run --frozen --no-python-downloads ruff check app tests
	cd frontend && npm run lint

typecheck:
	cd backend && uv run --frozen --no-python-downloads pyright
	cd frontend && npm run typecheck

test:
	cd backend && uv run --frozen --no-python-downloads pytest
	cd frontend && npm run test

hygiene:
	@! git ls-files | grep -E '(^|/)(\.env($|\.)|.*\.(pem|key|log)$$|data/|logs/|playwright-report/|test-results/)' | grep -v '\.env\.example$$' || (echo 'tracked secret/data/report artifact detected' >&2; exit 1)

check: lint typecheck test hygiene
	uv lock --project backend --check --no-python-downloads
	test -f frontend/src/api/index.ts
	cd frontend && npm run build
	docker compose --env-file deploy/.env.example -f deploy/compose.yaml config >/dev/null

docker-build:
	docker compose --env-file deploy/.env.example -f deploy/compose.yaml build api web

compose-up:
	docker compose --env-file deploy/.env.example -f deploy/compose.yaml --profile rabbitmq up -d rabbitmq
	@if curl --fail --silent http://127.0.0.1:9000/minio/health/live >/dev/null; then echo 'Reusing local MinIO on port 9000'; else docker compose --env-file deploy/.env.example -f deploy/compose.yaml --profile minio up -d minio; fi

compose-down:
	docker compose --env-file deploy/.env.example -f deploy/compose.yaml --profile rabbitmq --profile minio --profile identity down

identity-up:
	docker compose --env-file deploy/.env.example -f deploy/compose.yaml --profile identity up -d keycloak

identity-down:
	docker compose --env-file deploy/.env.example -f deploy/compose.yaml --profile identity down

contract:
	cd backend && LANVERSE_RUN_MINIO_CONTRACT=1 uv run --frozen --no-python-downloads pytest tests/contract/test_minio_port.py

contract-oidc:
	set -a; . deploy/.env.example; set +a; cd backend && LANVERSE_RUN_OIDC_CONTRACT=1 uv run --frozen --no-python-downloads pytest tests/contract/test_oidc_provider.py

e2e-install:
	cd frontend && npx playwright install chromium

e2e:
	cd frontend && npm run e2e
