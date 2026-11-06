FROM node:24.21.0-bookworm-slim AS base
WORKDIR /app
RUN corepack enable
COPY package.json pnpm-lock.yaml ./

FROM base AS build
RUN pnpm install --frozen-lockfile --ignore-scripts
COPY tsconfig.json ./
COPY src ./src
COPY types ./types
RUN pnpm build

FROM base
ENV NODE_ENV=production HOST=0.0.0.0 PORT=4400
RUN pnpm install --prod --frozen-lockfile --ignore-scripts
COPY --from=build /app/dist ./dist
USER node
EXPOSE 4400
CMD ["node", "--enable-source-maps", "dist/server.js"]
