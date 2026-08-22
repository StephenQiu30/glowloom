# Lanverse 管理端

Lanverse 管理端基于官方 [Ant Design Pro v6](https://github.com/ant-design/ant-design-pro) 构建，统一使用 pnpm 管理依赖。

当前页面范围：

- `/user/login`：用户登录
- `/user/register`：用户注册
- `/user/register-result`：注册结果
- `/admin`：管理入口
- `/account/settings`：账号设置
- `/exception/403`、`/exception/404`、`/exception/500`：系统错误页

国际化、语言切换及 Dashboard、表单、列表、详情、结果、Welcome、Chatbot 等官方示例已移除，后续只按 Lanverse 业务需要增加页面。

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

修改 `config/oneapi.json` 后执行 `pnpm run openapi` 重新生成 API 服务文件，不要直接修改 `src/services/ant-design-pro/`。
