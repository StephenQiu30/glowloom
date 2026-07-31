SHELL := /bin/sh
PYTHON ?= python3.11
VENV_PYTHON := backend/.venv/bin/python
PROD_ENV_FILE ?= .env.production
PROD_COMPOSE := docker compose --env-file $(PROD_ENV_FILE) -f docker-compose.yml -f docker-compose.prod.yml
PROD_EXAMPLE_COMPOSE := docker compose --env-file .env.production.example -f docker-compose.yml -f docker-compose.prod.yml

.PHONY: setup lock-backend dev-api dev-frontend scheduler worker-io worker-media db-init generate-api lint typecheck test hygiene check docker-build docker-dev-up docker-dev-down docker-dev-logs docker-prod-up docker-prod-down docker-prod-logs minio-up minio-down contract-minio contract-rabbitmq contract-ffprobe contract-media-stack contract-deepseek e2e-install e2e

$(VENV_PYTHON):
	@$(PYTHON) --version | grep -q 'Python 3.11.15'
	$(PYTHON) -m venv backend/.venv
	$(VENV_PYTHON) -m pip install 'pip==26.1.2'

setup: $(VENV_PYTHON)
	@! grep -q '^uv = ' backend/.venv/pyvenv.cfg || (echo 'backend/.venv was created by uv; recreate it with: python3.11 -m venv --clear backend/.venv' >&2; exit 1)
	@$(VENV_PYTHON) --version | grep -q 'Python 3.11.15'
	@node -v | grep -q 'v22.23.1'
	@npm -v | grep -q '10.9.8'
	$(VENV_PYTHON) -m pip install 'pip==26.1.2'
	$(VENV_PYTHON) -m pip install --requirement backend/requirements-dev.txt
	$(VENV_PYTHON) -m pip check
	cd frontend && npm ci

lock-backend: $(VENV_PYTHON)
	$(VENV_PYTHON) -m pip install 'pip==26.1.2' 'pip-tools==7.6.0'
	cd backend && .venv/bin/python -m piptools compile pyproject.toml --output-file requirements.txt --strip-extras
	cd backend && .venv/bin/python -m piptools compile pyproject.toml --extra dev --output-file requirements-dev.txt --strip-extras

dev-api:
	cd backend && .venv/bin/python -m uvicorn app.main:app --reload --host 127.0.0.1 --port 8000

dev-frontend:
	cd frontend && npm run dev

scheduler:
	cd backend && .venv/bin/python -m app.scheduler

worker-io:
	cd backend && .venv/bin/python -m app.io_worker

worker-media:
	cd backend && .venv/bin/python -m app.media_worker

db-init:
	cd backend && .venv/bin/python -m app.initialize_database

generate-api:
	cd frontend && npm run openapi2ts

lint:
	cd backend && .venv/bin/ruff check app tests
	cd frontend && npm run lint

typecheck:
	cd backend && .venv/bin/pyright
	cd frontend && npm run typecheck

test:
	cd backend && .venv/bin/python -m pytest
	cd frontend && npm run test

hygiene:
	@! git ls-files | grep -E '(^|/)(\.env($|\.)|.*\.(pem|key|log)$$|data/|logs/|playwright-report/|test-results/)' | grep -v -E '\.env(\.production)?\.example$$' || (echo 'tracked secret/data/report artifact detected' >&2; exit 1)

check: lint typecheck test hygiene
	$(VENV_PYTHON) -m pip check
	test -f frontend/src/api/index.ts
	cd frontend && npm run build
	docker compose --env-file .env.example config >/dev/null
	POSTGRES_PASSWORD=check-only RABBITMQ_DEFAULT_PASS=check-only MINIO_ACCESS_KEY=check-only MINIO_SECRET_KEY=check-only JWT_SECRET_KEY=check-only-production-secret $(PROD_EXAMPLE_COMPOSE) config >/dev/null

docker-build:
	docker compose build server web

docker-dev-up:
	docker compose up -d --build --wait

docker-dev-down:
	docker compose down

docker-dev-logs:
	docker compose logs --follow

docker-prod-up:
	@test -f $(PROD_ENV_FILE) || (echo 'copy .env.production.example to $(PROD_ENV_FILE) and configure production values' >&2; exit 1)
	$(PROD_COMPOSE) up -d --no-build --pull always --wait

docker-prod-down:
	$(PROD_COMPOSE) down

docker-prod-logs:
	$(PROD_COMPOSE) logs --follow

minio-up:
	@if curl --fail --silent http://127.0.0.1:9000/minio/health/live >/dev/null 2>&1; then \
		echo "MinIO is already available at 127.0.0.1:9000"; \
	else \
		docker compose up -d --wait minio; \
	fi

minio-down:
	docker compose stop minio

contract-minio:
	cd backend && LANVERSE_RUN_MINIO_CONTRACT=1 .venv/bin/python -m pytest tests/contract/test_minio_port.py tests/contract/test_media_minio_flow.py

contract-rabbitmq:
	cd backend && LANVERSE_RUN_RABBITMQ_CONTRACT=1 .venv/bin/python -m pytest tests/contract/test_rabbitmq_publisher.py tests/contract/test_rabbitmq_worker.py

contract-ffprobe:
	cd backend && LANVERSE_RUN_FFPROBE_CONTRACT=1 .venv/bin/python -m pytest tests/contract/test_ffprobe_media.py

contract-media-stack:
	cd backend && LANVERSE_RUN_MEDIA_STACK_CONTRACT=1 .venv/bin/python -m pytest tests/contract/test_media_stack.py

contract-deepseek:
	@test -n "$${DEEPSEEK_API_KEY:-}" || (echo 'DEEPSEEK_API_KEY is required for the real DeepSeek E2E contract' >&2; exit 1)
	cd frontend && LANVERSE_RUN_DEEPSEEK_E2E=1 npx playwright test

e2e-install:
	cd frontend && npx playwright install chromium

e2e:
	cd frontend && npm run e2e
