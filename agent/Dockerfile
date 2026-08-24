ARG PYTHON_IMAGE=python:3.11-slim-bookworm
FROM ${PYTHON_IMAGE}

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PYTHONPATH=/srv/lanverse/agent

WORKDIR /srv/lanverse/agent

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install --no-install-recommends --yes curl ffmpeg \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system lanverse \
    && useradd --system --gid lanverse --create-home lanverse

COPY requirements.txt ./requirements.txt
RUN python -m pip install --requirement requirements.txt

COPY --chown=lanverse:lanverse app ./app

USER lanverse

EXPOSE 8686

HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=12 \
    CMD curl --fail --silent http://127.0.0.1:8686/healthz >/dev/null || exit 1

CMD ["python", "-m", "app.runtime.api"]
