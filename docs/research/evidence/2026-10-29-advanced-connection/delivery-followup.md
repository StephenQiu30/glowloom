# 多选连接交付验证补记

对应已推送提交：`ae85b5f4`。候选基于 `90485fdb48ec00a09cfbe518b030a6f788c272ee` 在独立目录验证；16 个代码/测试候选的 SHA256 与最终 manifest、实际构建目录逐字节一致。本补记仅归档验证事实，不将高级节点、异常恢复或真实服务验收记为完成。

## 执行结果

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| 候选专项测试 | 7 文件、63 测试通过 | 候选 manifest 中记录；本次无独立专项日志归档，不冒充全量结果 |
| `npx next build` | exit 0；默认 Turbopack，完整路由表 | `delivery-next-build.log` / `delivery-quality-status.json` |
| `npx next typegen`、`npx tsc --noEmit --pretty false` | 均 exit 0 | `delivery-typegen.log` / `delivery-tsc.log` / `delivery-quality-status.json` |
| `npx eslint .` | exit 0 | `delivery-eslint.log` / `delivery-quality-status.json` |
| `npx vitest run --maxWorkers=2` 首次全量 | **exit 1**；13 文件失败、159 文件通过；35 测试失败、1102 测试通过；779.90 秒 | `delivery-full-vitest.log` / `delivery-full-vitest-status.json` |
| 仅失败的 13 文件，以 `--maxWorkers=1` 串行复核 | exit 0；13 文件、157 测试全部通过；31.81 秒 | `delivery-failed-files-serial.log` / `delivery-failed-files-serial-status.json` |
| `npx prettier --check .` | **exit 1**；10 文件格式告警 | `delivery-prettier.log` / `delivery-quality-status.json` |

## 首次失败与复核边界

首次全量运行期间观察到宿主机还有其他任务的 Vitest、构建/类型检查等负载。本轮完整保留失败终态，不把运行中、首次失败或后续复核改写成“首次全量通过”。串行复核使用相同候选、相同依赖、原测试超时；未修改产品代码、断言或超时，也没有同时执行本轮构建/lint。所有失败文件复核通过，当前未留下稳定复现的测试失败；并发负载与失败的因果关系仍不能仅凭这次复核确证。

Prettier 的 9 个非 globals 告警文件与候选基线 HEAD 逐字节相同；HEAD 的 globals.css 提取后，使用相同 `.prettierrc.json` 检查也返回 exit 1。证据为 `delivery-prettier-baseline-identities.json`、`delivery-prettier-head-globals.log`。未扩大范围修复历史格式问题；16 个代码候选均不在告警列表。

JSON 中的 `/tmp` 路径保留原始执行来源；归档副本均位于本目录并使用 `delivery-` 前缀。验证说明不替代 CI 终态、真实主体/时间线领域事务或服务端接受与重建验收。
