# Lanverse Frontend

前端使用 Next.js App Router 与 TypeScript。页面按 View/ViewModel 组织，HTTP 统一经过 `src/lib/request.ts`，接口文件由 `@umijs/openapi` 从 `backend/docs/swagger.json` 生成。

```bash
npm ci
OPENAPI_SCHEMA_URL=../backend/docs/swagger.json npm run openapi2ts
npm run dev
```

质量门禁：

```bash
npm run lint
npm run typecheck
npm run test
npm run build
```

本地服务地址和完整剧本解析流程请参阅仓库根目录 `README.md`。
