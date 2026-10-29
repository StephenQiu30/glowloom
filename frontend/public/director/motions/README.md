# Quaternius 角色动作

作者：Quaternius。来源为 [Universal Animation Library](https://quaternius.itch.io/universal-animation-library) 的免费 **Standard** 包，依据包内 [CC0 1.0 许可](./Quaternius-CC0.txt) 使用。官方介绍：[Quaternius Universal Animation Library](https://quaternius.com/packs/universalanimationlibrary.html)。

2026-10-29 通过公开匿名免费下载入口获取 `Universal Animation Library[Standard].zip`（15,904,933 字节，itch.io upload ID `17958403`）。未获取付费 Pro / Source 包。归档 SHA256：`cc73fc4e495b82958207316596317a3f40b9fa38065bde1027937452da537724`。

从包内 `Unreal-Godot/UAL1_Standard.glb`（7,618,436 字节，SHA256 `69591853d817488edaa8fd9bf8fc1d821eaeaf789f8627b3cd23b41c4ed67997`）提取全部 **43 条真实动画**的旋转轨道（包括待机、行走、慢跑、冲刺、蹲姿、坐姿、跳跃、游泳、互动、舞蹈等）；目录由 `director-motion-catalog.ts` 列举，数量由实际资源校验。

`quaternius-ual1-standard.json` 保存65骨源静止旋转和43条动画原始float32关键帧/时间；不含网格、材质、平移、缩放。可复现提取脚本为 `frontend/scripts/extract-director-motions.py`，参数为已许可原GLB与目标JSON路径。没有招手动作或即梦私有742条素材。

运行时在目标角色完成体型调整、尚未播放动画时，将 `targetRest × inverse(sourceRest) × sourceFrame` 写入新的四元数轨道，并归一化。轨道绑定到该显示实例的骨骼 UUID。必须在私有显示克隆上调用，不能在附加后再克隆并原样复用轨道。保留目标骨长、位置和缩放；角色位移由导演台路径控制。这是旋转适配，不包含脚底接触约束或完整动作捕捉重定向，实际视觉效果仍须验收。

不附带 FBX、付费资源或原始网格。此目录为公开静态资源，不引入新的用户数据持久化。
