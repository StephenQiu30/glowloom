SHELL := /bin/sh

.PHONY: setup check

setup:
	@uv --version | grep -q '0.11.32'
	@node -v | grep -q 'v22.23.1'
	@npm -v | grep -q '10.9.8'
	uv sync --project backend --frozen --no-python-downloads
	cd frontend && npm ci

check:
	uv lock --project backend --check --no-python-downloads
	cd frontend && npm run lint
	cd frontend && npm run build
