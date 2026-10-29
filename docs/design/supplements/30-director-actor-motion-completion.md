# 导演台角色与动作补充合同（DIR-009/022/023/024）

## 1. 摘要
扩展左右肢体参数、可复用真实动作库、独立本地动作导入、姿势片段及相邻混合。本文件独立于设计30正文；所有实现仍由导演台场景数据持久化，保留双工作区、读写权限和512KiB上限。

## 2. 参与者
导演台编辑者可创建和应用；只读用户可查看与临时预览。身体修改仅静态工作区。导入/预览/混合确认均以对象ID与场景基线保护取消或过期写入。

## 3. 背景与证据
即梦实证：research/13及evidence/2026-10-29-director/static-pose-properties.txt、actor-animation-library-loaded.txt、new-animation.txt。身体整体、胸腰、头部、左右肢体实证是姿势角度；独立肢体长度是用户本轮明确要求的扩展，不冒称竞品实证。BeefTV固定2327ee2d80282b10dd8bef0f26b48990d46f5c3b（MIT）提供轮廓变形与Mixer基础，不提供可直接复制的742动作。已下载Quaternius Standard CC0包真实43动作，沿用source-rest到target-rest旋转适配。

## 4. 目标
原10项体型新增leftArm/rightArm/leftLeg/rightLeg，遗漏侧别值继承原arms/leg。左右骨架和网格同一变形映射，身体改变清冻结快照。姿势提供标准身体分组但仅显示实际映射骨骼。

## 5. 使用场景
动作库提供搜索、预设/我的、取消式临时预览、播放头应用；我的来自对象motion_library持久数据。独立GLB/glTF/FBX/BVH文件解析为兼容骨骼轨道并保存，可撤销失败且不生成虚假资产ID。保存姿势将实际骨骼姿态转换为恒定片段，可用于当前兼容骨架。文字生成只有实际受支持provider才启用，不用随机或已有动画冒充。

## 6. 价值
43条真实CC0动作与用户可复用动作使用同一Mixer采样。复制/拖动/裁剪延续已有source_offset、倍率和循环合同，不制造虚假数量或不可执行控件。

## 7. 数据与执行合同
- body_profile新增四个可选侧别参数，范围仍0.25–1.6。
- motion_library为对象可选数组，每项{id,name,kind:import|pose,duration,tracks,rest}。track为{bone,property:position|quaternion|scale,times,values}，rest按骨名保存局部变换。每对象最多16来源、每来源256轨道/8192采样点，总场景仍512KiB，超限明确拒绝而不静默截断。不含URL/可执行内容/图片或模型网格。
- 兼容须目标骨骼唯一且轨道骨骼逐一存在。旋转用targetRest×inverse(sourceRest)×sourceQuaternion；位置以源rest位移差适配目标骨长，缩放保留目标rest比例。不能映射、不支持的轨道明确报错。
- source_animation=user:<sourceID>引用我的来源；姿势源是0与duration两点恒定轨道。删除已使用源须先移除片段。库来源在static→dynamic同步时不得丢失原动态片段引用。
- blend_in为片段起点之后的相邻过渡秒数，默认0。只在前片段结尾与本片段起点相接且0<blend_in<=min(前时长,本时长)时有效。不重叠时间轴片段：过渡期间前片段冻结末帧权重1-a，本片段按原source_offset/rate/loop采样权重a。第三片段和不相邻链不隐式混合。取消混合设0。移动/裁剪破坏邻接时拒绝，不能悄悄删混合。
- 新增职责组件负责库和导入/姿势/混合交互；scene/viewport/workspace由集成方接线。预览回调只改变临时view scene，不进入持久历史。

## 8. 验证与未决服务
须测试左右独立、骨架/网格一致、43目录真实加载、导入兼容/不兼容/超限/取消、持久往返、姿势恒定与后续重建、混合边界/取消/裁剪保护和只读。文字生成调查范围为当前catalog/operation/providerhttp实现；截至本轮核对只有JSON/图像/视频/音频/none输出及文本、图像、视频、语音适配器，未发现骨骼动画输出与motion生成适配。不能据此宣称外部云服务宕机；可执行后续接口为新增motion.generate能力、异步作业/取消/结果骨架合同，或部署具备许可证的text-to-motion引擎后接同一导入合同，需真实接口与样本验收。


### 本轮实际服务核验与可执行接入选项

- 当前源码 `backend/internal/catalog/domain/model_registry.go:35–62` 的输出类型为json/image/video/audio/none，未定义骨骼动作产物。
- `backend/internal/operation/adapter/providerhttp/client.go:106–135` 仅处理video.generate/text.generate/image.generate，其他能力返回unknown；语音是单独适配分支。
- `backend/internal/catalog/domain/provider_preset.go` 的预设能力白名单没有motion；对backend/frontend业务源码检索text-to-motion、3d motion、HyMotion、BVH相关生成器未发现生产适配。此结论是代码能力核验，不是外部服务宕机或账号额度结论，也没有读取凭据。
- 选项A（当前可用）：导入兼容GLB/glTF/FBX/BVH，或符合DirectorMotionSource的动作JSON；经过骨架、采样与大小校验后成为“我的动作”。非骨骼轨道、非线性插值或不兼容骨名明确拒绝。
- 选项B（待真实后端接入）：新增motion.generate能力与异步作业，输入{prompt,model_release_id,skeleton,fps,duration,seed?}，返回{id,status}；查询返回queued/running/completed/failed/cancelled，completed必须给来源信息/许可和可验证的DirectorMotionSource或自包含GLB/BVH，再走现有导入校验。取消为幂等接口。现有通用文本JSON输出不能据此被当作已验收的物理动作生成。
- 选项C（待部署）：接入具有明确许可、硬件条件和样本验收的本地text-to-motion引擎，实现选项B同一合同。当前仓库没有该服务部署或真实动作模型，因此本轮不创建假任务、不返回假成功。

### 集成接口

`DirectorMotionLibrary({object,info,dynamic,time,duration,disabled,onChange,onPreview,onCapturePose})`：onPreview({object,time})只替换临时视图中的对应角色，null恢复；不写历史。info.rest是加载时的实际基础骨骼变换。onCapturePose读取当前已采样模型的实际骨骼快照。预览取消/对象基线变化取消，不污染动态轨道。

`DirectorPoseEditor({object,rest:info.rest,disabled,onChange})`：显示真实映射骨骼的7组相对角度，以加载rest乘当前姿势delta为基准（默认0°），写回base乘relative。局部修改和组重置只写对应bone_override并保留其他冻结姿势。静态身体比例编辑仍清理冻结快照。只读禁止写但允许临时动作预览。

动作预览按对象基线与动作名缓存同一个预览对象；逐帧仅更新时间，避免每帧重建GLB模型。

补充回归：导演台目录26文件173测试通过；动作库专项10测试含真实GLB/BVH导入、相对角度与预览引用稳定回归。Go专项5个顶层测试使用race通过。最新动作库/姿势组件和专项测试ESLint通过，git diff --check通过。全仓tsc当前被并行文件`src/app/api/[...path]/route.ts:686`的query联合类型访问阻断。

实机导入夹具`frontend/tests/fixtures/director/quaternius-walk-import.glb`包含CC0原始Walk_Loop的65骨骼轨道稀疏采样，65,656字节，动作名`Walk_Loop_CC0_Import`；解析后持久数据小于512KiB。夹具验证不代表FBX所有导出器兼容或真实服务生成验收。

本轮资源为43条，原GLB SHA256为69591853d817488edaa8fd9bf8fc1d821eaeaf789f8627b3cd23b41c4ed67997；导出JSON 12,914,262字节，含CC0来源字段，原始float32轨道无伪造。实例共享已校验资源读取；URL版本化避免旧3条资源缓存命中。旋转适配无脚底接触IK，视觉验收由主任务在实机进行。
