.PHONY: test-architecture test-database-design test-migration test-contract-foundation test-contract contracts-toolchain-check generate-api contracts-check test-jobs test-integration test-e2e test lint typecheck build build-images build-render-image verify-render-image

PNPM_NODE = pnpm dlx node@24.18.0 $$(command -v pnpm)

test-architecture:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/architecture -q -p no:cacheprovider

test-database-design:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/database_design -q -p no:cacheprovider

test-migration:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/migrations -q -p no:cacheprovider

test-contract-foundation:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/contract/test_http_foundation.py -q -p no:cacheprovider

test-contract:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/contract -q -p no:cacheprovider

contracts-toolchain-check:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/contract/test_umi_toolchain.py -q -p no:cacheprovider

generate-api:
	cd backend && uv run python scripts/generate_api_client.py

contracts-check: generate-api
	git diff --exit-code -- frontend/src/api

test-jobs:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/jobs -q -p no:cacheprovider

test-integration:
	cd backend && PYTHONDONTWRITEBYTECODE=1 uv run pytest tests/integration -q -p no:cacheprovider

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

build-render-image:
	docker build --file backend/Dockerfile.render --tag lanverse-render:noto-2.004 backend

verify-render-image: build-render-image
	LANVERSE_RENDER_IMAGE=$$(docker image inspect --format '{{.Id}}' lanverse-render:noto-2.004) \
		uv run --project backend python backend/scripts/verify_render_runtime.py
