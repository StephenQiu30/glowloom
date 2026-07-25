.PHONY: test-architecture test-database-design test-migration test-contract-foundation contracts-toolchain-check contracts-check test-jobs test-e2e test lint typecheck build build-images

PNPM_NODE = pnpm dlx node@24.18.0 $$(command -v pnpm)

test-architecture:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/architecture -q -p no:cacheprovider

test-database-design:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/database_design -q -p no:cacheprovider

test-migration:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/migrations -q -p no:cacheprovider

test-contract-foundation:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/contract/test_http_foundation.py -q -p no:cacheprovider

contracts-toolchain-check:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/contract/test_umi_toolchain.py -q -p no:cacheprovider

contracts-check:
	cd backend && uv run python scripts/export_openapi.py --check
	cd frontend && $(PNPM_NODE) run openapi:generate
	git diff --exit-code -- backend/openapi/openapi.json frontend/src/services/generated

test-jobs:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/jobs -q -p no:cacheprovider

test-e2e:
	cd frontend && $(PNPM_NODE) exec playwright test

test: test-architecture
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest -q -p no:cacheprovider
	cd frontend && $(PNPM_NODE) test

lint:
	cd backend && uv run ruff check .
	cd frontend && $(PNPM_NODE) lint

typecheck:
	cd backend && uv run mypy
	cd frontend && $(PNPM_NODE) typecheck

build:
	cd backend && uv build
	cd frontend && $(PNPM_NODE) build

build-images:
	docker compose -f docker-compose.yml build
