# Lanverse Backend

Python 3.13 FastAPI API and PostgreSQL TaskJob worker for the Lanverse MVP.
The package exposes only the `lanverse-api` and `lanverse-worker` commands once
the composition roots are added by PLAN-02 P02-T003.

Dependencies and the interpreter are managed exclusively by uv:

```bash
uv sync --frozen
uv run pytest tests/architecture
```
