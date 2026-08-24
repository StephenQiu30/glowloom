# Local Codex Storyboard Agent Harness Implementation Plan

日期：2026-08-24

1. 审计现有 Shot/Draft/Worker 契约与两个上游研究快照，把适用概念映射为项目自有规则，不引入运行时仓库依赖。
2. 删除 DeepSeek 运行时入口，接入本机 Codex CLI structured output。
3. 建立五个通用英文 Skill，保持显式调用、candidate-only、无副作用。
4. 以失败测试固定 Harness schema、硬门、有限修复、组装和恢复契约。
5. 实现 `storyboards/agents`、数据库 checkpoint、可续租 run lease 与 fencing，并接入 IO worker。
6. 扩充 Provider/ShotSpec 映射与关键分镜表 CSV/HTML 导出。
7. 增加 Reviewer policy、资产 Tool gate 与 Agent 领域目录架构测试。
8. 先跑单场景真实 Codex 合约，再跑两场景、对白、跨空间动作和关键道具合约。
9. 执行 Storyboard/数据库回归、全后端测试、前端检查、Ruff、Pyright、Skill 校验和工作区审计。

当前交付边界为“固定脚本输入 → 本地 Codex 多阶段候选 → 硬门/审核/修复 → checkpoint → 关键分镜表 → needs_review”。图片生成进入后续独立 Design，不创建未来空目录。
