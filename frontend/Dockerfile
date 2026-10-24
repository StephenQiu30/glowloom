# syntax=docker/dockerfile:1
FROM node:24.21.0-bookworm-slim AS base
ENV PNPM_HOME=/pnpm PATH="/pnpm:$PATH" NEXT_TELEMETRY_DISABLED=1
RUN corepack enable
WORKDIR /app

FROM base AS deps
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
COPY canvas-collab/package.json /canvas-collab/package.json
COPY canvas-collab/src /canvas-collab/src
RUN pnpm install --frozen-lockfile

FROM deps AS dev
ENV NODE_ENV=development
COPY frontend/ .
EXPOSE 3000
# Named node_modules volumes outlive images. Refresh file: packages from the
# mounted source before Next resolves their exports, without changing the lockfile.
CMD ["sh", "-c", "pnpm install --frozen-lockfile && exec pnpm exec next dev --hostname 0.0.0.0 --port 3000"]

FROM base AS build
# Next persists external Upgrade rewrites in its build manifest.
ARG GLOWLOOM_BACKEND_ORIGIN
COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /canvas-collab /canvas-collab
COPY frontend/ .
RUN test -n "$GLOWLOOM_BACKEND_ORIGIN" && pnpm exec next build

FROM node:24.21.0-bookworm-slim
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 HOSTNAME=0.0.0.0 PORT=3000
WORKDIR /app
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
COPY --from=build --chown=node:node /app/public ./public
USER node
EXPOSE 3000
CMD ["node", "server.js"]
