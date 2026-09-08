ARG PYTHON_IMAGE=python:3.11-slim-bookworm
ARG NODE_IMAGE=node:22-bookworm-slim
FROM ${NODE_IMAGE} AS node-runtime
FROM ${PYTHON_IMAGE}

ARG CODEX_CLI_VERSION=0.149.1
ARG TARGETARCH

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PYTHONPATH=/srv/lanverse/agent

WORKDIR /srv/lanverse/agent

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install --no-install-recommends --yes ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system lanverse \
    && useradd --system --gid lanverse --create-home lanverse

COPY --from=node-runtime /usr/local/bin/node /usr/local/bin/node
COPY --from=node-runtime /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
    && case "$TARGETARCH" in arm64) CODEX_ARCH=arm64 ;; amd64) CODEX_ARCH=x64 ;; *) exit 1 ;; esac \
    && npm install --global "@openai/codex@${CODEX_CLI_VERSION}" \
       "@openai/codex-linux-${CODEX_ARCH}@npm:@openai/codex@${CODEX_CLI_VERSION}-linux-${CODEX_ARCH}" \
    && codex --version \
    && npm cache clean --force

COPY agent/requirements-creation.txt ./requirements-creation.txt
RUN python -m pip install --requirement requirements-creation.txt

COPY --chown=lanverse:lanverse agent/skills ./skills
COPY --chown=lanverse:lanverse agent/app/__init__.py agent/app/main.py ./app/
COPY --chown=lanverse:lanverse agent/app/api ./app/api
COPY --chown=lanverse:lanverse agent/app/core ./app/core
COPY --chown=lanverse:lanverse agent/app/skills ./app/skills
COPY --chown=lanverse:lanverse agent/app/harness ./app/harness
COPY --chown=lanverse:lanverse agent/app/modules ./app/modules
COPY --chown=lanverse:lanverse agent/app/reasoning ./app/reasoning
COPY --chown=lanverse:lanverse agent/app/protocol ./app/protocol
COPY --chown=lanverse:lanverse agent/app/creation ./app/creation

COPY --chown=lanverse:lanverse agent/app/text_contract ./app/text_contract
RUN mkdir -p /home/lanverse/.codex && chown lanverse:lanverse /home/lanverse/.codex
USER lanverse

EXPOSE 8787

HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=12 \
    CMD curl --fail --silent http://127.0.0.1:8787/readyz >/dev/null || exit 1

CMD ["uvicorn", "app.main:create_app", "--factory", "--host", "0.0.0.0", "--port", "8787"]
