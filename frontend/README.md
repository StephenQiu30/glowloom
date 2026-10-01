# Lanverse 前端

工作台完整迁移正在执行。当前源码采用 Next.js App Router、React、TypeScript strict、Tailwind、shadcn/ui（Radix）、TanStack Query 和 Zustand；首页、项目、素材、表单生成、任务、模型配置与无限画布已接入自身 Go API。批量表格、导演台、GLB、图片工具和时间轴的迁移状态以 [正式设计](../docs/design/BeefTV能力引入设计.md) 为准，页面存在不等于完整能力或真实模型验收完成。

从仓库根目录的既有配置启动 Go API 后，在此目录运行：

```bash
pnpm install --frozen-lockfile
LV_API_BASE_URL=http://127.0.0.1:8080 pnpm exec next dev --hostname 127.0.0.1 --port 3000
```

本地工作区继续使用无登录的持久身份，模型管理遵循服务端权限。浏览器请求通过同源 `/api` 转发，生成客户端只消费后端在线 Swagger；不能以源项目的 Wails、本地数据库或供应商接口替换此链路。

```bash
LV_API_BASE_URL=http://127.0.0.1:8080 pnpm exec openapi2ts
pnpm exec prettier --ignore-path /dev/null --write src/gen/api/*.ts
pnpm exec eslint .
pnpm exec prettier --check .
pnpm exec next typegen
pnpm exec tsc --noEmit
pnpm exec vitest run
pnpm exec next build
```

交互验收使用实际服务与浏览器；需要外部模型、费用、对象存储或工作流的能力分别保留真实证据。单元测试、类型检查与构建不能代替这些验收。目录与质量要求见 [PROJECT.md](../PROJECT.md) 和 [AGENTS.md](../AGENTS.md)；许可入口为 `/licenses`。
