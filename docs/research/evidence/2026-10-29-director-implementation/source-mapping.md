# 导演台实现来源与项目适配

固定上游：[BeefTV 2327ee2](https://github.com/glanderness/BeefTV/tree/2327ee2d80282b10dd8bef0f26b48990d46f5c3b)，该提交为 MIT；采用本地固定快照核对，没有直接跟随新版 main。MIT 全文保留于 `frontend/src/components/canvas/director/notices/BeefTV-MIT.txt`。以下是算法和行为的消化整理，不是整套应用复制。

| 上游 `web/src/lib/canvas/director/` | Glowloom 整理位置 | 项目化变化 |
| --- | --- | --- |
| director-scene.ts | director-animation.ts、director-model.ts | 当前DTO与有限值校验；位置/旋转/缩放、target/FOV/roll独立；Quaternion插值；各通道曲线互不覆盖 |
| director-actor-paths.ts、director-camera-paths.ts | director-paths.ts、director-path-sampling.ts、director-path-editor.tsx | 米制路径采样、帧时间、预览确认/取消；覆盖检查；自由运镜先目标再机位 |
| director-camera-binding.ts | director-scene.ts、director-cameras.ts | 机位/目标世界与舞台坐标转换，保持现有稳定ID与引用清理合同 |
| director-gesture-transaction.ts | director-gesture.ts、director-history.ts | 预览/提交分离，锁定与只读，两个工作区独立历史，最终一次Yjs CAS |
| director-resources.ts | director-resources.ts、director-viewport.tsx | 有界模板缓存、私有模型克隆；abort生命周期；geometry/material/texture/skeleton/shadow释放 |
| director-save.ts、director-session.ts | 现有yjs-live-session.ts与director-workspaces.ts | 只借鉴异步身份/版本语义；仍使用项目Yjs权威保存与epoch、服务确认，未引入上游store/保存接口 |
| 输出状态恢复思路 | director-video-output.ts、director-capture.tsx、director-video-dialog.tsx | mediabunny确定性逐帧编码、产物解码验证；接项目媒体上传/转码/引用；失败保留重试 |

数据领域新增V2静态/动态工作区，兼容V1。后端canvas domain负责容量、引用与字段校验；Postgres引用检查/复制清理跨两个工作区；生成API由现有命令重新生成。前端使用项目shadcn/Radix、Lucide和token。未引入BeefTV AntD界面、Zustand业务store、AI后端、品牌或即梦私有发布代码。

角色为Quaternius Standard男女CC0模型，12个体型由同一基础骨架和几何共同变形。动作单独使用官方Universal Animation Library Standard CC0资源的43条动作；资源README和许可证随 `public/director/motions/` 保存。重定向只保留骨骼旋转，保留目标骨长和体型；路径负责位移，不宣称脚底约束或高级步态匹配。

本地GLB/glTF/FBX/OBJ通过Three官方加载器解析并转换成自包含GLB，再使用项目上传接口。自建OBJ正式页面验证仅证明这一格式与该文件的完整闭环；FBX/glTF动画单元解析证据不等于任意第三方模型兼容。


第二轮补充：角色资产为12种体型的实际渲染缩略图；简模为Kenney CC0的32个模型，按家具、建筑、自然、街道、厨房、卫浴、办公、日用归类；13个几何体以实际网格生成缩略图。每项资源的归一化尺寸、来源和许可随目录清单保留，不复制即梦的私有目录数量或内容。

独立动作导入将GLB/glTF/FBX/BVH/动作JSON转换为项目有界骨骼TRS与sourceRest合同；动作库、姿势保存、相邻混合、逐对象临时预览使用当前Director文档和历史事务。源rest读取先于动作采样，避免将当前姿势再次当作重定向基准。文字生成动作尚无真实骨骼动画服务，界面明确显示不可用。

拒绝稿恢复沿用项目权限与服务端存储：按用户、画布、节点隔离，保存基线指纹和epoch，以单调revision做CAS；30天逻辑保留，不在浏览器持久存储业务文档。备份失败保留会话内容并提供恢复文件下载。该能力是Glowloom现有协议适配，不引入上游本地草稿存储。

第二轮实现和真实页面边界见[验证记录](validation-round2.md)，Figma更新见[42状态回读](figma-round2.md)。
