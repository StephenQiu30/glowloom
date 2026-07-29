SHELL := /bin/sh

.PHONY: setup dev-api dev-frontend scheduler db-init generate-api lint typecheck test hygiene check docker-build business-up business-down minio-up minio-down env-up env-down contract-minio contract-rabbitmq e2e-install e2e

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

scheduler:
	cd backend && uv run --frozen --no-python-downloads python -m app.scheduler

db-init:
	cd backend && uv run --frozen --no-python-downloads python -m app.initialize_database

generate-api:
	cd frontend && npm run openapi2ts

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
	docker compose -f docker-compose.yml config >/dev/null
	docker compose -f docker-compose-env.yml config >/dev/null

docker-build:
	docker compose -f docker-compose.yml build api web

business-up:
	docker compose -f docker-compose.yml up -d

business-down:
	docker compose -f docker-compose.yml down

minio-up:
	@curl --fail --silent http://127.0.0.1:9000/minio/health/live >/dev/null 2>&1 || \
		docker compose -f docker-compose-env.yml up -d minio

minio-down:
	docker compose -f docker-compose-env.yml stop minio

env-up:
	docker compose -f docker-compose-env.yml up -d

env-down:
	docker compose -f docker-compose-env.yml down

contract-minio:
	cd backend && LANVERSE_RUN_MINIO_CONTRACT=1 uv run --frozen --no-python-downloads pytest tests/contract/test_minio_port.py

contract-rabbitmq:
	cd backend && LANVERSE_RUN_RABBITMQ_CONTRACT=1 uv run --frozen --no-python-downloads pytest tests/contract/test_rabbitmq_publisher.py

e2e-install:
	cd frontend && npx playwright install chromium

e2e:
	cd frontend && npm run e2e
