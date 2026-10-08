# Lanverse 前端

工作台使用 Next.js App Router、React、TypeScript、shadcn/Radix、TanStack Query 和 Zustand。目录、工具链与质量要求见 [PROJECT.md](../../../../../PROJECT.md)；业务合同见 [工作台设计](../../../design/工作台设计.md)。

启动 Go API 后，在此目录运行：

```bash
pnpm install --frozen-lockfile
LV_API_BASE_URL=http://127.0.0.1:8080 pnpm exec next dev --hostname 127.0.0.1 --port 3000
```

浏览器通过同源 `/api` 访问 Go API。业务客户端由 `@umijs/openapi` 从在线 Swagger 统一生成到 `src/api/`，调用 `src/lib/request.ts` 的 Axios 封装；禁止手写业务请求、接口路径或修改生成文件。上传代理与授权媒体流复用同一传输入口。完整请求、权限、容量和取消约定见 [DES-03 §1.1](../../../design/03-接口设计.md#11-公共-api-文档与前端请求链)。

生成与检查：

```bash
LV_API_BASE_URL=http://127.0.0.1:8080 pnpm exec openapi2ts
pnpm exec prettier --ignore-path /dev/null --write 'src/api/*.ts'
pnpm exec eslint .
pnpm exec prettier --check .
pnpm exec next typegen
pnpm exec tsc --noEmit
pnpm exec vitest run
pnpm exec next build
```

CI 从在线规范重新生成并核对整个 API 目录。交互变化补充浏览器验证；模拟测试和构建不能替代真实媒体、供应商、费用及完整产品验收。许可入口为 `/licenses`，来源见 [THIRD_PARTY_NOTICES](../../../../../THIRD_PARTY_NOTICES.md)。
