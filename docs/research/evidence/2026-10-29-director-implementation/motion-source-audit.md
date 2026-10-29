# 角色动作来源与接入核验（2026-10-29）

BeefTV 固定版本：2327ee2d80282b10dd8bef0f26b48990d46f5c3b。软件 LICENSE 为 MIT；第三方资产遵循独立许可。

## 固定代码与已有模型

- `web/src/lib/canvas/director/director-scene.ts:425` 列出21种静态姿势；`directorPoseBoneDeltas:429` 返回固定骨骼四元数。walk/run/wave等名称不表示时间动画。
- `web/src/components/canvas/director/canvas-director-workbench.tsx:1246` 的handleActorRigReady从导入模型AnimationClip创建motionClips；`director-viewport.tsx:1429` 创建Mixer动作、1438按时间采样。未发现独立内置人体动作资产库。
- `director-actor-paths.ts:76` 等生成对象位移轨迹，不生成行走骨骼周期。
- BeefTV与Glowloom当前男女Quaternius GLB完全同字节，每份3 mesh、1 skin、65 joints、0 animations。男性SHA256：96d4cac60b335184d623b4217cf1dfeb6113c785a1a9e8ae4684799970dc2ec0；女性：d3c96c2af4423580cf8ebf07595a60f816c92280743a785e101acdfb943ebf39。
- Glowloom 12角色是2性别×6体型，由两份基础GLB体型变形而来，均无原生时间动画。facecap.glb虽有4条动画，但无人体蒙皮，不适用本任务。

## 已核验的实际资产

公开来源：https://quaternius.itch.io/universal-animation-library；官方说明：https://quaternius.com/packs/universalanimationlibrary.html。

采用公开匿名免费下载的Standard包，无账号、支付或付费版本访问。`provenance.json`记录归档哈希；原包`License.txt`明确CC0 1.0。归档15,904,933字节，非root-motion GLB为7,618,436字节，43条实际动画。具体清单在animation-manifest.json。Idle_Loop 2.5秒、Walk_Loop约1.333秒、Jog_Fwd_Loop约0.933秒、Sprint_Loop约0.667秒；免费包没有wave，不伪造。

源与目标65骨名完全一致，但静止旋转、骨长及比例不相同。直接套用195条TRS轨道会把目标体型覆盖成源体型。

## 已按后续授权交付的最小实现

产品资源仅包含Idle_Loop/Walk_Loop/Jog_Fwd_Loop。public/director/motions/quaternius-ual1-standard.json保留65源rest quaternion与195条总计旋转轨道，1,184,895字节，gzip250,190字节；附原包Quaternius-CC0.txt与来源README。没有网格、平移、缩放或生成动作。

attachBuiltinDirectorMotions(root,signal)在目标私有克隆完成体型变形、播放前读取当前rest，按targetRest×inverse(sourceRest)×sourceFrame适配旋转。骨长、位置、缩放不变，root位移交给路径。轨道绑定克隆骨骼UUID，不可先给共享template添加后直接克隆轨道。实现复用Three AnimationClip/QuaternionKeyframeTrack与现有root.animations，无新持久化。

8项测试通过，包括12种真实角色体型×3动作采样、骨长/缩放不变、未知骨拒绝、取消/共享加载、已有clip保留与幂等附加。ESLint/Prettier通过。全仓TypeScript在并行director-body.test.tsx:59的body_profile字段报错，未修改该无关文件。未进行浏览器视觉/真服务验收；此适配不含脚底约束IK，不能称为完整动作捕捉重定向。
