PNPM ?= pnpm
UV ?= uv
FRONTEND := frontend
BACKEND := backend

.PHONY: bootstrap build dev lint test-architecture typecheck

bootstrap:
	$(PNPM) --dir $(FRONTEND) install --frozen-lockfile
	$(UV) sync --directory $(BACKEND) --frozen

build:
	$(PNPM) --dir $(FRONTEND) build
	$(UV) run --directory $(BACKEND) python -m compileall -q apps packages tools

dev:
	$(PNPM) --dir $(FRONTEND) dev

lint:
	$(PNPM) --dir $(FRONTEND) lint
	$(UV) run --directory $(BACKEND) ruff check apps packages tests tools

typecheck:
	$(PNPM) --dir $(FRONTEND) typecheck
	$(UV) run --directory $(BACKEND) mypy apps packages tools

test-architecture:
	PYTHONDONTWRITEBYTECODE=1 $(UV) run --directory $(BACKEND) \
		python -m unittest discover \
		-s tests/architecture -p 'test_*.py'
