# Lanverse 前端

Next.js（App Router）+ TypeScript strict + Tailwind CSS + shadcn/ui（Radix）。目录与约定见仓库根目录 [PROJECT.md §6](../PROJECT.md)。

```bash
pnpm install --frozen-lockfile
node --env-file=../.env "$(command -v corepack)" pnpm exec next dev  # http://localhost:3000
pnpm exec eslint .
pnpm exec prettier --check .
pnpm exec next typegen
pnpm exec tsc --noEmit
pnpm exec vitest run
pnpm exec next build
```
