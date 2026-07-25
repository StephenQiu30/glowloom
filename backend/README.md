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
