# syntax=docker/dockerfile:1
FROM node:24.21.0-bookworm-slim AS base
ENV PNPM_HOME=/pnpm PATH="/pnpm:$PATH" NEXT_TELEMETRY_DISABLED=1
RUN corepack enable
WORKDIR /app

FROM base AS collaboration
WORKDIR /canvas
COPY canvas/package.json canvas/pnpm-lock.yaml canvas/tsconfig.json ./
RUN pnpm install --frozen-lockfile --ignore-scripts
COPY canvas/src ./src
COPY canvas/types ./types
RUN pnpm build

FROM base AS deps
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
COPY canvas/package.json /canvas/package.json
COPY --from=collaboration /canvas/dist /canvas/dist
COPY --from=collaboration /canvas/types /canvas/types
RUN pnpm install --frozen-lockfile

FROM deps AS dev
ENV NODE_ENV=development
COPY frontend/ .
COPY backend/internal/prompt/domain/cinema_catalog.json /backend/internal/prompt/domain/cinema_catalog.json
EXPOSE 3000
CMD ["pnpm", "exec", "next", "dev", "--hostname", "0.0.0.0", "--port", "3000"]

FROM base AS build
# Next persists external Upgrade rewrites in its build manifest.
ARG GLOWLOOM_BACKEND_ORIGIN
COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /canvas /canvas
COPY frontend/ .
# Type checking includes the test that compares the frontend and backend catalogs.
COPY backend/internal/prompt/domain/cinema_catalog.json /backend/internal/prompt/domain/cinema_catalog.json
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
