# JC-002 节点统计与项目信息验收回执

来源：`advanced-source/project-information.txt/png`、`project-credits.txt/png`。主任务读取的稳定modal为800×546；1869×873观察视口中x534.5/y163.5。统计3非零类别时200×172。设计唯一合同为设计27 §7.4.15，PRD14 AC-CJC-01，Plan23 JC-002补充。

## 实现

- 统计浮层固定200px，只列非零，图片/视频/音频/文本/时间线/主体/外部顺序，其他单列；导演台与CinemaKit归外部。传入完整CanvasEditor nodes，堆叠隐藏成员仍计数。基础页分布与源一致八格，将外部/容器/未知合并其他。
- 基础信息通过正式 `GET /canvases/{id}/information` 同一授权快照读取：workspace所属项目owner展示名，canvas.document创建/更新时间。缺失owner显示信息不可用，不拿当前用户顶替。
- billing模块按operation冻结canvas来源连接settle账本，sum使用十进制字符串避免精度损失，只计已确认任务；reserve/release/provider_overage、未确认与其他画布不会混入。明细每页20条按时间/ID排序，并具备上下页、错误重试和关闭/身份变动后迟到结果废弃。BFF仅允许GET及page参数。
- 现有账本单位是CNY微元，没有积分转换合同，页面明确实际费用（人民币），保留源tab与明细入口位置。AI对话draft来源尚不计入画布费用，提示范围。此为全量一致性残差，不以伪造积分绕过。

## 验证

- 真实本轮隔离PostgreSQL18，`TestCanvasInformationScopedLedgerPaginationAndMetadata`，`-race` passed（2.481s）：真实owner/时间、空集合0、21条扣款分页无重复、跨空间/不存在/已删拒绝、分页参数验证、四类账本隔离。该数据为隔离测试事实，不是用户正式付费账单。
- 前端information/BFF/theme共3files80tests通过；真实Radix交互覆盖非零顺序、精确微元、数据失败不显示假0、重试、项目身份不匹配、关闭abort、明细分页。
- 相关ESLint、GoVet、GoLint通过（0issues），govulncheck 0reachable/0 imported，1unused required module警告；不将未达调用路径的模块警告描述为代码漏洞。
- 最终全仓tsc通过；全画布+theme回归65files367tests全部passed（exit0）。助手测试Radix卸载后异步focus任务现在等待当前JSDOM realm完成，广范围未再出现unhandled Event错误。根任务更晚的全仓检查为最终结果。
- 正式浏览器读取/刷新与非零用户账单未由本子任务验证，等待主任务独立回执。

## Figma原生稿

[总览](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3149-53373)；[稳定截图](figma-project-information.png)。在原page1361:10保留原验收节点身份更新：统计3041:46003（200×172）与3041:46078（空态200×100）；信息1618:21476/1618:21530（各1869×873），内部modal3149:53516/3149:53700（各800×546）。总览位于x238000/y40。

读回原生层：首次包含72文本/114Frame/48实例/112Vector，后补三个类型图标增加11个原生节点；图片填充0。复用既有Canvas背景/header/dock/viewport/AI与ghost Button主组件、canvas-reference变量、Noto Sans SC字体；截图没有作为UI图层导入。画板数据为注明示例，缺失头像不复制源账户头像。该设计读回不等同产品浏览器/真实结算验收。
