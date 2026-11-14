# Workspace 使用与部署指南

本指南维护长期操作合同，实际接受与环境见对应 Plan 和 BACKLOG。静态与在线使用同一知识正文，但分别构建、打包和部署。

## 1. 初始化与静态阅读

在仓库的 `workspace/` 目录安装锁定依赖，执行 `pnpm build:static`，再执行 `pnpm serve:static`。默认只监听 `127.0.0.1:1131`，不会启动业务 API、PG、Redis、Kafka、Codex 或 TTS。

构建输入包含允许范围的正文、根规范、Skill、流程及可选音频。输出为 `dist/static/<release_id>/`，其 `release.json` 记录来源、范围、音频指纹和资源摘要清单。`current.json` 仅指向上次成功产物；构建失败不会替换它。脏工作区产物标为本地预览，不代表已发布提交。

生产静态托管直接使用完整产物目录，可使用任意 HTTP 静态服务器。需要内部访问控制时保护 HTML、raw、Pagefind、图表、音频和 cue 的全部路径。公共产物需先在 catalog 中明确允许范围，执行 `pnpm build:static --public`；不能通过隐藏侧栏或 noindex 代替权限。

独立域名／根路径优先。子路径构建前设置 `WORKSPACE_BASE_PATH=/knowledge`，托管层统一映射该前缀；不能在产物生成后仅修改浏览器链接。深链接、404、中文路径和音频 Range 访问须在目标托管环境验证。

## 2. 在线启动与身份

执行 `pnpm build:online` 后，以 `pnpm start:online` 启动。启动器监听 `127.0.0.1:1130`，创建本机私有令牌和受控桥；令牌文件为 `.local/access-token`，权限为0600，不打印到日志或发送到模型。可通过运行环境设置 `GLOWLOOM_WORKSPACE_SESSION_SECRET`，不把它写入构建参数或 `NEXT_PUBLIC_*`。

浏览器输入令牌后取得30分钟 HttpOnly、SameSite=Strict 会话。写请求还需同源和会话绑定的 CSRF。退出不删除已发生的运行事实；轮换令牌并重启在线服务可使原会话失效。首版仅本机 owner，团队身份提供方未接入，不使用产品账号自动授予研发权限。

`GLOWLOOM_WORKSPACE_ORIGIN` 必须与浏览器真实来源一致；默认 `http://127.0.0.1:1130`。反向代理和HTTPS部署显式配置来源及转发，Cookie在HTTPS来源下启用Secure。端口可用 `WORKSPACE_PORT` 配置，不占用现有1122／1123业务端口。

## 3. PostgreSQL 与权限

现有业务已经使用 `workspace` schema，研发运行因此使用独立的 `dev_workspace`。结构和对象权限只定义在 `backend/db/schema.sql`，包括提案、运行、事件、产物引用和执行绑定。应用启动不执行DDL。

新环境由管理员提供原有规范角色，再对空数据库执行根 schema；公共业务预设按需要导入 data。现有库由管理员离线比对并受控变更，不能重放空库定义，不能新增增量SQL文件。

管理员另建专用LOGIN并授予受限 `glowloom_workspace` 能力角色，凭据离线配置；不授予 `glowloom_app` 或业务schema权限。连接通过 `GLOWLOOM_WORKSPACE_DB_DSN` 在运行时注入。服务检查本角色可访问研发schema且不能访问产品workspace；权限过宽时协作台拒绝持久化操作。

未配置或PG失联时，受控知识快照仍可阅读，写入与运行显示不可用。运行保留期和生产容量在部署前核定；不得把缺失监控或未运行验收补为零或通过。

## 4. 编辑、主机与交接

协作台先读取当前原文，再保存带基线摘要的提案。审阅修改前后后单独应用；已有修改导致冲突时保留提案，不覆盖正文。应用、Git提交、合并和站点发布互相独立；在线正文落地不会自动更新已发布阅读快照。

受控桥通过0600 Unix socket与独立令牌接收已限定动作，只接受catalog中的Markdown文档与允许工程ID。在线运行包不需要挂载可写仓库；真正写入发生在本机桥拥有的指定工作树。

首个主机为本机安装的Codex app-server接口。`GLOWLOOM_WORKSPACE_CODEX_BINARY`可指定已核对的二进制；默认优先使用桌面应用附带CLI。任务默认只读；显式允许写入时仅开放选定工程路径，关闭工具网络和外部MCP／Apps，交互批准不会被自动同意。不得提交、推送、发布或收费。

任务固定代码、知识、Skill、流程及授权版本。阶段依赖、实际输出、命令证据和人工接受分别留记录。断连、重启或未知回执先查询原run／attempt／外部句柄；无法查询则等待人工核对，不自动重派。输入变化后明确复核上下文，旧证据保留并标记需重验。

受控命令包括Workspace的unit／content／typecheck，frontend的`pnpm exec tsc --noEmit`、agent／canvas的`pnpm typecheck`、backend的`go vet ./...`。结果只证明对应检查，不代替业务集成或产品接受；不接收任意shell字符串。原始执行日志在私有产物存储，知识正文只保留必要证据引用。

## 5. 朗读资源导入

真实TTS生成暂不启用。已有合法音频可通过以下命令导入：

```bash
pnpm import:narration /absolute/path/manifest.json --authorized
```

导入器要求ffprobe可用，用于核对实际音频时长。manifest必须绑定documentId、revision、scope、configDigest、precision与segments；每片包含id、相对音频路径、sha256、durationMs和cues。每条cue包含毫秒起止、blockId及源文本范围。块ID可从对应阅读页的 `data-block-id` 或内容解析结果取得。

manifest的scope须匹配目标internal／public；正文公开不自动授权音频公开。导入、替换或撤除音频会改变产物身份。音频相对路径不能越界或指向符号链接；每片不超过100MB，摘要和实测时长必须一致。只有实际可靠的时序才能定位，无句级数据时使用段落／片段或纯播放，不均分字数伪造逐字对齐。

导入只放入 `.local/narration/<document_id>/`，不立即发布。重新构建时将匹配资源归入同版产物；旧文档或配置不匹配即阻止使用。私有音频／cue与正文同等保护，不包含声音样本、凭据或供应商临时URL。

## 6. 验证、发布与恢复

类型、lint、单元、内容和构建均通过后，再在实际静态／在线环境验证。数据库测试显式配置 `GLOWLOOM_WORKSPACE_TEST_DB_DSN`，只接受名称以 `glowloom_workspace_test_` 开头的隔离库；没有配置时结果是跳过，不是数据库接受。

两版分别核对产物模式、资源清单、来源、范围和访问保护。在线standalone需显式包含启动器、依赖、`.next/static`、必要public及受控快照；不得借构建工作区补文件。静态回退整套原产物，在线回退先验证schema和执行接口兼容；不反向改表，不清空运行事实，不回滚业务代码或数据。

正式发布、域名变更、生产库操作、Git提交和推送按当时授权执行。本机预览、测试容器或构建成功不能代替生产发布和产品接受。


## 7. 容器与整包选版

`deploy/Dockerfile.static`接收独立构建上下文的`site/`完整静态产物和`workspace/deploy/nginx.conf`，只运行Nginx8080。子路径由托管层按构建basePath映射。`deploy/Dockerfile.online`接收`package/`完整standalone产物和`workspace/deploy/online-entry.mjs`，只运行Node1130。构建上下文只复制这些允许文件，不从仓库根发送凭据或其他工程。

在线镜像不包含可写仓库或主机桥。运行环境注入身份、PG和桥配置；Unix socket位于可信同机边界，0600所有者UID应与容器运行UID一致，不公开桥端口或挂整个仓库。跨主机桥和团队身份属于后续部署设计；未配置时协作能力不可用。

选版使用`pnpm release:select static <release-id>`或`pnpm release:select online <release-id>`，校验模式与完整清单后原子更新指针，再重启对应进程。回退不改另一模式、正文或数据库，不手工补已发布资源。CI在`.github/workflows/workspace.yml`执行隔离PG、质量、双构建与静态浏览器；只有远端实际运行结束才可声称CI通过。
