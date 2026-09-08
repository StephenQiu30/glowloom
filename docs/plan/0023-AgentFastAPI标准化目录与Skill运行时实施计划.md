# Agent FastAPI 标准化目录与 Skill 运行时实施计划

- 状态：本轮实施
- 依据：[设计](../design/0023-Agent%20FastAPI%20标准化目录与%20Skill%20运行时设计.md) 与 [需求](../requirement/0023-AgentFastAPI标准化目录与Skill运行时需求规格.md)

## 清单

- [x] 建立 `app/main.py` 工厂、`app/core` 配置/容器/生命周期。
- [x] 建立 `app/api/dependencies.py` 和 `app/api/routes/`，迁移所有生产 HTTP 路由。
- [x] 删除业务模块中的 FastAPI 应用、闭包路由安装和生命周期篡改。
- [x] 将 `candidate_runtime` 归一为 `harness` 业务包，并通过 HarnessService 暴露执行用例。
- [x] 将 SkillCatalog 注入运行时容器、HarnessService 和 readiness。
- [x] 更新 Docker、CI、测试和 README，移除旧入口引用。
- [x] 完成静态边界、HTTP、合同、质量门禁和镜像冒烟检查。

## 验证命令

```sh
cd agent
.venv/bin/ruff check app tests
.venv/bin/ruff format --check app tests
.venv/bin/pyright app tests
.venv/bin/pytest -q
cd ..
docker compose config --quiet
docker build --file agent/Dockerfile --tag lanverse/agent:fastapi-standard .
```
