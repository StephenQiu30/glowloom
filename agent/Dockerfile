ARG PYTHON_IMAGE=python:3.11-slim-bookworm
ARG NODE_IMAGE=node:22-bookworm-slim
FROM ${NODE_IMAGE} AS node-runtime
FROM ${PYTHON_IMAGE}

ARG CODEX_CLI_VERSION=0.147.0

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
    && npm install --global "@openai/codex@${CODEX_CLI_VERSION}" \
    && npm cache clean --force

COPY agent/requirements.txt ./requirements.txt
RUN python -m pip install --requirement requirements.txt

COPY --chown=lanverse:lanverse agent/skills ./skills
COPY --chown=lanverse:lanverse agent/app ./app

USER lanverse

EXPOSE 8787

HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=12 \
    CMD curl --fail --silent http://127.0.0.1:8787/healthz >/dev/null || exit 1

CMD ["uvicorn", "app.candidate_runtime.api:app", "--host", "0.0.0.0", "--port", "8787"]
