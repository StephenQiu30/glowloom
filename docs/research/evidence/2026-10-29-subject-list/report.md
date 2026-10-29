# 即梦主体列表加载与设计收敛

日期：2026-10-29；目标：已登录Chrome中的 `/ai-tool/elements`。本次读取当前页面实际加载的脚本，不沿用前一日旧hash结论。

## 结论与证据

| 问题 | 结论 | 证据类型与边界 |
| --- | --- | --- |
| 是否瀑布流 | 否。等高CSS Grid，封面16:9；1512px视口4列，每列292px，网格gap8px，卡片内部padding4px | 实时DOM，jimeng-dom.json、jimeng-card.json；列数为该视口观测 |
| 是否一次性加载全部 | 否。固定20条分批获取 | 实测POST `/mweb/v1/dreamina_subject/get`，cursor:0、limit:20；脱敏network-summary.json |
| 是否滚动加载 | 底部IntersectionObserver在可见比例达到0.2时调用onLoadMore，loading/error状态阻止自动重入 | 当前已加载Elements静态代码，static-excerpts.json；无臆测的“瀑布流”命名 |
| 后续批次与结束 | 响应next_cursor与has_more；只有hasMore/loadingMore/error存在才渲染尾部；加载spinner、失败重试、结束不显示额外文案 | 实测响应字段 + 静态渲染分支 |
| 查询与主体页附加内容 | 没有可见页码、每页数量、总页数、历史主体入口、产品页脚 | 当前登录页DOM与用户截图 |
| 多批真实运行是否验证 | 未验证：此账号只有1个主体，has_more:false，无法自然触发第二批 | 不以静态代码或本地fixture冒充即梦第二批实测 |

REA Evidence：`ev_37196e7282bce29a93c9f82cfaf3713ec1244af638563334dc876a16a4040bb3`，父记录 `ev_32b6d8e7f359e913b62b506a0ba93e245cc49b259676b3f8099eb90b02bf7b19`。两份脚本282739 bytes，148模块，无parse failure；semantic graph覆盖为partial，动态关联仍未知。分析未执行bundle bootstrap。实时浏览器通过现有Chrome扩展的CDP捕获，REA负责所捕获原始脚本的静态分析；没有宣称REA直接完成运行时捕获。

## 采用的实现边界

织光复用现有page/page_size/total API，固定20条，只在前端累积网格。服务端仍先筛选排序后分批；不新增重复游标接口或虚拟瀑布流框架。offset在并发增删下可能移动，按ID去重不代表快照完整性。空批停止；整批重复仍继续下一批，以total和递增页码限定请求范围，避免offset移动时提前隐藏后续内容。筛选/搜索/身份/空间变化取消旧请求并清空旧累计结果；保存、收藏、删除后刷新首批；追加失败保留旧项且重试原批。观察器接受祖先滚动裁剪，页面和弹窗均可使用。

主体主页移除分页、历史入口及页脚；旧路由、版本及历史引用保留。营销、会员、品牌等不在主体列表复制范围。其余列表视觉差异不得以本轮加载测试通过统称为全量像素一致。

## 验证

- Red：原实现18项测试中5项失败，原因包括仍请求12条、没有底部检测器。
- Green：主体列表20项通过；最终主体域16个文件104项通过；精确文件ESLint和Prettier通过。
- 扩展回归：21个文件131项，130通过；资产顶栏“参考素材”断言失败，发生于并行资产变更，不属本轮主体修改。
- 初次类型检查与浏览器被并行资产文件 `asset-template-list.tsx:80` 缺少右括号阻塞；后续复验见下文。
- Figma同步和读回见figma-list-alignment.json。

### 最终验证补充

- `pnpm --dir frontend exec vitest run tests/unit/components/subjects --maxWorkers=2`：16文件104测试通过。补充整批重复继续读取、同帧重复触底去重、追加后收藏刷新首批。
- agent-browser隔离会话：20→40→45追加；独立Dialog 20→40；失败保留20条、重试同一页后40条，请求序列1/2/2；390px下到45条且无主体区域横溢。axe主体区域0违规/0未完成。详情见browser-verification.json。
- 真实Chrome正式入口1122：已有1个主体；1869×873与390×844确认无分页、历史入口、产品页脚或横向溢出。未生成/修改真实主体。
- 初次资产语法阻塞已随并行修改消失。生产构建编译通过，TypeScript仍在并行`canvas-assistant-panel.tsx:210`的getOperation缺少project_id处失败；独立tsc曾在并行canvas-subject-reference测试的exact/media类型处失败。不将这些失败报告为通过，不覆盖相应未提交文件。
- React复查：请求生命周期清理、查询身份隔离、函数式追加/去重、无重复全库请求；编辑器仍动态加载；只用现有组件及原生IntersectionObserver，未引入新依赖。
- 本轮只收敛主体列表加载方式与已明确的额外UI；卡片间距/图标和其他功能不能据此声称逐像素全量一致。

## 提交后卡片与语义收敛（SUB-016）

按用户顺序，既有列表改动先以`91978177`提交并推送main，随后更新需求12、设计25、PRD12、计划21，再落实卡片实现。此节覆盖前述旧构建阻塞状态，保留原始检查记录。

- 桌面网格gap8px、卡片padding4px、封面圆角8px、信息12px/20px；收藏改为右上星标且收藏后常显，更多操作移到信息区。颜色使用根主题语义token。
- 创建排序改读created_at并展示“创建”，修改排序保留updated_at和“修改”；红测2项创建排序断言失败，修复后主体16文件104项通过。日期筛选不再隐藏新建入口。
- agent-browser独立45条fixture验证：新建/普通卡片标题同高，创建排序文案、菜单可达；390px无横溢，收藏及更多44px，日期条件下新建保留。主体区域axe0违规/0未完成。详情见[卡片验证](card-verification.json)。这次卡片验收使用fixture，不冒充正式后端新数据或即梦第二批实测。
- 全量ESLint、tsc和生产构建已通过；此前画布类型阻塞已消失。全量单测该轮1300通过/4失败：两项画布连线断言、一项资产顶栏断言和主题策略。主题策略中的本轮颜色问题已修复并复验，剩余画布两处硬编码颜色；不覆盖并行画布/资产文件或宣称全绿。
- 未引入新依赖、虚拟列表或通用加载框架；当前组件继续复用Button、DropdownMenu与共享顶栏。
- Figma原位同步4个卡片状态、44个实例和46个网格，并读回桌面/手机/日期状态；未改公共Header/Button。手机更多保持44px，未补造手机菜单页。详见[设计读回](figma-card-alignment.json)，设计节点验证不代替浏览器运行。
