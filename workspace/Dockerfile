# syntax=docker/dockerfile:1
FROM node:24.21.0-bookworm-slim
ENV PNPM_HOME=/pnpm PATH="/pnpm:$PATH" NEXT_TELEMETRY_DISABLED=1
RUN corepack enable
WORKDIR /repo/workspace
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY . .
EXPOSE 3000
CMD ["pnpm", "exec", "next", "dev", "--webpack", "--hostname", "0.0.0.0", "--port", "3000"]
