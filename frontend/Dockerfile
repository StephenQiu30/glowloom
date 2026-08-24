ARG NODE_IMAGE=node:22-alpine

FROM ${NODE_IMAGE} AS dependencies
WORKDIR /srv/lanverse/frontend
COPY package.json package-lock.json ./
RUN npm ci

FROM ${NODE_IMAGE} AS builder
WORKDIR /srv/lanverse/frontend
COPY --from=dependencies /srv/lanverse/frontend/node_modules ./node_modules
COPY . .
ARG NEXT_PUBLIC_API_BASE_URL=http://127.0.0.1:8686
ENV NEXT_PUBLIC_API_BASE_URL=${NEXT_PUBLIC_API_BASE_URL}
RUN npm run build

FROM ${NODE_IMAGE} AS runtime
ENV NODE_ENV=production \
    HOSTNAME=0.0.0.0 \
    PORT=8123
WORKDIR /srv/lanverse/frontend

RUN addgroup --system --gid 1001 nodejs \
    && adduser --system --uid 1001 nextjs

COPY --from=builder --chown=nextjs:nodejs /srv/lanverse/frontend/public ./public
COPY --from=builder --chown=nextjs:nodejs /srv/lanverse/frontend/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /srv/lanverse/frontend/.next/static ./.next/static

USER nextjs

EXPOSE 8123

HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=12 \
    CMD wget --quiet --spider http://127.0.0.1:8123/ || exit 1

CMD ["node", "server.js"]
