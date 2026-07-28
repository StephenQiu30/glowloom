SHELL := /bin/sh

.PHONY: setup dev-api dev-frontend db-init lint typecheck test hygiene check

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

lint:
	cd backend && uv run --frozen --no-python-downloads ruff check app tests
	cd frontend && npm run lint

typecheck:
	cd backend && uv run --frozen --no-python-downloads pyright
	cd frontend && npx tsc --noEmit

test:
	cd backend && uv run --frozen --no-python-downloads pytest

hygiene:
	@! git ls-files | grep -E '(^|/)(\.env($|\.)|.*\.(pem|key|log)$$|data/|logs/|playwright-report/|test-results/)' | grep -v '\.env\.example$$' || (echo 'tracked secret/data/report artifact detected' >&2; exit 1)

check: lint typecheck test hygiene
	uv lock --project backend --check --no-python-downloads
	cd frontend && npm run build
