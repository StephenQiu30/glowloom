SHELL := /bin/sh

.PHONY: setup dev-api dev-frontend db-init generate-api lint typecheck test hygiene check e2e-install e2e

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

e2e-install:
	cd frontend && npx playwright install chromium

e2e:
	cd frontend && npm run e2e
