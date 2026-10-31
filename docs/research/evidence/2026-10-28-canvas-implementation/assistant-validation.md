# 画布助手会话与结果采用验证

日期：2026-10-29（Asia/Shanghai）。来源：设计27 §7.4.5、PRD14 AC-CJC-18、Plan23 CASE-CJC-18，以及 advanced-parity-contracts.md §5。源实页证明400px可调侧栏、参考入口、七技能名称及说明，不证明七技能内部执行流程。assistant-skills.txt明确视频反解/创作分镜输出为视频/分镜Prompt，广告技能完整名称为全流程广告片导演。

## 可执行合同与检查

- 会话服务位于 canvas/application/assistant.go、adapter/postgres/assistant.go、adapter/http/assistant.go；当前用户+画布+项目重新授权。会话不进入Yjs图，不随项目复制；过期项目内容清理同时清空私有会话内容。
- 稳定会话UUID、草稿UUID与CAS修订控制创建/绑定；同草稿的并发跨会话绑定串行化，只能成功一次。绑定确认前不挂可发送的正式编辑器。
- 续聊只接终态任务，服务端固定实际operation；previous_operation_id与读取回复的任务不一致时拒绝，避免用旧上下文续聊新任务。关闭submission slot后仍从永久submission找回任务。
- 正式RichGeneration承担模型、参考、报价、确认、取消、未知恢复；没有移除admin ai-trials鉴权，也没有使用本地消息数组伪装会话后端。
- 后端保留海报和商品图片的image执行合同与已有轮次恢复；源侧栏以自然语言讨论开始，五快捷与七技能菜单当前写入正式草稿。已移除旧非源“专业模式”切换，尚无从讨论自动编排到图片/视频任务的执行器，不能宣称海报/套图技能已交付。已审核图片结果续聊前重新读取媒体身份/版本，再转为固定参考。
- 独立采用owner在await前捕获epoch，重新读取正式结果/媒体后建立新节点；保存确认前不报成功。保存未知重试复用同节点，恢复epoch后的迟到结果拒绝。
- BFF只开放3种精确UUID路由及对应方法，禁止通过actor/owner查询扩大范围。首次浏览器404由缺少BFF有界路由导致，已加回归覆盖。

## 实际运行

| 命令/环境 | 观察结果 | 证据边界 |
| --- | --- | --- |
| 独立PG18库 glowloom_canvas_assistant_tests，受限canvas_test_runtime角色；go test -race ./tests/canvas -run '^TestAssistant' -count=1 | 3项通过，3.202s，0 skip | 持久化、权限、CAS、竞态、任务身份冻结；operation记录为合成fixture，未调用供应商 |
| pnpm exec vitest run assistant-state / assistant-panel / assistant-adoption | 3文件16项通过 | 固定身份/审核边界、图片引用、绑定前不挂生成器、未知重试去重、迟到epoch拒绝；API为fixture |
| BFF route.test.ts | 69项通过 | HTTP代理方法/路径/查询围栏 |
| pnpm exec tsc --noEmit | 通过 | 此次共享工作区整包类型检查 |
| scoped ESLint | 0错误 | assistant目录 |
| go vet ./internal/canvas/... | 通过 | canvas包 |
| golangci-lint run ./internal/canvas/... ./tests/canvas/... | 0 issues | 当前共享canvas代码与测试 |
| govulncheck ./internal/canvas/... | 调用路径0漏洞 | 报告依赖模块有1项未调用漏洞，不能写成依赖整体无漏洞 |

## 当前开发库增量DDL

经根任务授权，以backend指定GLOWLOOM_DB_DSN仅在内存连接当前开发库，未输出或保存凭据。运行角色有canvas schema CREATE权限。只执行事务内CREATE TABLE IF NOT EXISTS、CREATE INDEX IF NOT EXISTS及SELECT/INSERT/UPDATE授权；没有全量schema重置、删除数据或变更其他表。新增表8列均NOT NULL，主键及owner索引存在，应用角色权限检查通过，迁移后初始0行。

审查文件：assistant-additive-schema.sql；回读：assistant-additive-schema-receipt.txt。根任务负责统一重启与嵌入画布的浏览器验证。

## 仍缺少的证据与完整范围

没有执行真实模型对话或收费生成；合成operation和API测试不证明供应商可用。完整七技能专业流程、视频理解、结构化分镜采用、多镜头导演/短片、品牌设计成套成果、新建/管理技能、与源会话消息布局逐像素对照仍需独立闭环。海报/商品图片的一次实际生成，也不能替代套图一致性或源专业技能完整验收。

当前代码没有将上述缺口标记为全量一致。设计27保留七技能逐项输入、执行、产物与采用合同，进度仍由根BACKLOG维护。

## 联调修复及新增回归

根任务实际浏览器发现助手PromptEditor内部flex项宽度0，根任务在正式PromptEditor补w-full/min-w-0/flex-1后已输入并保存草稿r2；显式reload后重开同草稿正文已通过（browser-assistant-draft-reopened.png）。不是本模块API单测能够替代的视觉验收。

标签保存丢失独立定位：真实CanvasYjsLiveSession的change→Yjs字节→serviceProjection→权威snapshot/ack可以保存colorTags。CanvasEditor原applyingRemote屏障仅在nodes/connections引用与替换值严格相等时清除，媒体预览抢先替换数组会使屏障持续吞掉后续本地修改。修复记录前置render引用，仅忽略旧render和精确远端echo，后续本地数组正常提交。完整live-session50项、标签3项、时序2项，共55项通过、0skip；根任务浏览器已确认青绿/紫色触发保存、等待已保存后刷新仍保留标签且正文不变。

助手颜色移到globals的canvas语义tokens，焦点1px；theme-policy 3项通过，助手无局部独立调色板或2px焦点圈。

## 助手紧凑呈现与空会话阶段（同日补充）

按advanced-source/assistant-panel.png/txt补 `presentation="assistant"`，保留prompt_draft所有者；400px侧栏四边12px留白、24px圆角，中央五项专业创作快捷，底部正文与+/技能/@/设置/发送。生成模型、参数、保存状态在设置弹层；费用与任务仍经正式确认对话框。草稿为空时发送禁用，正在准备或绑定未知时不会挂可发送编辑器。参考缩略详情可从+菜单的“本次参考”进入，未把reference_image等内部字段放到空态。

打开后自动恢复已有空草稿；相同用户/画布只允许一个未发送会话。仅quoted的operation不算发消息，accepted或closed+acknowledge_result才允许新增；并发创建以事务锁拒绝第二个空会话。列表恢复当前实际任务，切换会话时迟到operation回调按draft_id围栏。这里只限制空会话，不删除已有会话或未保存输入。

回归：generation-dialog、reference-mention-menu、unified-mention、canvas-subject-reference、assistant-panel、theme-policy共6文件50项通过；新增用例用真实RichGeneration/Composer校验assistant呈现的input_source仍为prompt_draft、模型选择与正式报价，确认调用前不发送。API为fixture，不能等同provider证明。另3项PG race全部通过，覆盖报价仍为空会话、已发送后开放、列表操作恢复、并发双空会话拒绝和跨画布同草稿绑定争用。TypeScript与作用域ESLint通过。

根任务真实浏览器记录：browser-assistant-compact.png；r2原草稿恢复、400px布局、+/技能/@/设置/发送、新建空会话禁用均可见。设置→生成模型为空并显示“暂无匹配的可用模型”；没有可用模型，未发出真实provider任务。七技能目录和快捷写入不是专业工具执行：视频理解、分镜Prompt质量、全流程广告、短片制作、套图/海报自动执行、品牌成套结果与技能创建/管理仍未完成，不能标为全量一致。

最终合并定向检查：12文件185项通过（助手16、标签/远端时序/live55、BFF69、主题3、正式generation-dialog31、三组参考组件11）；TypeScript与作用域ESLint均0错误，git diff --check通过。后端最后一次3项PG race通过、golangci-lint 0 issues。测试不代表真实provider可用或全量专业功能一致。
