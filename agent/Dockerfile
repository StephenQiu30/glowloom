# syntax=docker/dockerfile:1
FROM python:3.12.14-slim AS build
COPY --from=ghcr.io/astral-sh/uv:0.11 /uv /usr/local/bin/uv
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy UV_PYTHON_DOWNLOADS=never
WORKDIR /app
COPY pyproject.toml uv.lock .python-version ./
RUN uv sync --locked --no-dev --no-install-project
COPY app ./app

FROM python:3.12.14-slim
RUN useradd --system --uid 10001 lanverse
WORKDIR /app
COPY --from=build /app /app
ENV PATH="/app/.venv/bin:$PATH" PYTHONUNBUFFERED=1
USER 10001
EXPOSE 8090
CMD ["uvicorn", "app.main_api:create_app", "--factory", "--host", "0.0.0.0", "--port", "8090"]
