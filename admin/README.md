# Lanverse Admin

Lanverse 管理端使用官方 [Ant Design Pro v6](https://github.com/ant-design/ant-design-pro) 作为 UI 基线，并统一使用 pnpm 管理依赖。

当前保留的页面：

- `/user/login`：用户登录
- `/user/register`：用户注册
- `/user/register-result`：注册结果
- `/admin`：管理入口
- `/account/settings`：账号设置
- `/exception/403`、`/exception/404`、`/exception/500`：系统错误页

国际化、语言切换、Dashboard、表单、列表、详情、结果、Welcome 和 Chatbot 等官方示例已移除，后续按 Lanverse 业务模块逐步加入。

## 开发

```bash
pnpm install --frozen-lockfile
pnpm run dev
```

## 验证

```bash
pnpm run lint
pnpm test
pnpm run build
```

OpenAPI 规格位于 `config/oneapi.json`。修改规格后使用 `pnpm run openapi` 重新生成 `src/services/ant-design-pro/`，不要直接修改生成文件。
