# Frontend scaffold

本项目的前端基线由 Vercel/Next.js 官方 CLI 生成：

```bash
npx --yes create-next-app@16.2.12 frontend \
  --typescript --tailwind --eslint --app --src-dir --empty \
  --use-npm --disable-git --no-agents-md --yes
```

生成后的 `src/app`、TypeScript、ESLint、Tailwind、`@/*` 别名与 npm lockfile 约定保持不变；Lanverse 在该基线上增加 shadcn/ui、业务页面、OpenAPI client 和测试。项目级启动与环境说明仍以仓库根 `README.md` 为准。
