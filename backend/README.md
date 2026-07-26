# Lanverse Backend

Python 3.13 FastAPI API and PostgreSQL TaskJob worker host for the Lanverse MVP.
The package exposes only the `lanverse-api` and `lanverse-worker` commands.

Dependencies and the interpreter are managed exclusively by uv:

```bash
uv sync --frozen
uv run pytest tests/architecture
uv run lanverse-api
uv run lanverse-worker
```

Both processes validate `LANVERSE_` settings at their entrypoint. The API app
factory performs no network or filesystem I/O during import.

The source tree uses one FastAPI-oriented technical layering model without an
extra project-name package wrapper:

```text
src/
├── main.py / worker.py
├── api/routes/          # HTTP mapping only
├── schemas/             # Pydantic and job payload contracts
├── services/            # use cases and transaction orchestration
├── repositories/        # parameterized asyncpg SQL and row mapping
├── domain/              # framework-independent values and state machines
├── workers/             # TaskJob lease and handler execution
├── integrations/        # approved provider adapters
├── db/                  # connection primitives
└── core/                # settings, lifecycle, logging, and runtime
```

FastAPI serves the only OpenAPI contract at `/openapi.json`. From the repository
root, `make generate-api` starts a loopback API when needed and lets
`@umijs/openapi` regenerate the frontend client directly from that URL. No static
OpenAPI copy is stored in the repository.
