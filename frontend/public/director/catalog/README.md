# 导演台简模目录与许可

32 个真实模型，8 类各 4 个；这是 Glowloom 选定的公开资源子集，不是即梦私有简模目录的复制。目录可在 `manifest.json` 逐项核对原文件、源 SHA-256、转换后 SHA-256、来源、分类与尺寸。

来源为作者 Kenney 官方公开下载：

- [Furniture Kit 2.0](https://kenney.nl/assets/furniture-kit)：家具、建筑、厨房、卫浴、办公、日用，以及长椅和垃圾桶。
- [Nature Kit](https://kenney.nl/assets/nature-kit)：自然、木栅栏和路牌。

两份下载包均自带 Creative Commons Zero (CC0) 许可，原文原样保存在 `furniture-kit-CC0.txt` 与 `nature-kit-CC0.txt`。允许个人、教育、商业使用；署名不强制。我们保留作者和来源，未使用付费下载或任何即梦素材。

下载验证于 2026-10-29：

| 官方公开下载包    |     字节数 | SHA-256                                                          |
| ----------------- | ---------: | ---------------------------------------------------------------- |
| furniture-kit.zip |  5,130,729 | e67652d0932cee41683f74711c03d3e192a2af9979ef8e6b237711f5482d46b0 |
| nature-kit.zip    | 10,537,521 | fa7974a0d342bfe63c38664ba9f8ec1a4aab8ea25f099bdc56870e33588c4d9d |

## 模型处理

源 GLB 为自包含 glTF 2.0，无外部图片或缓冲区引用。保留原网格、材质和节点层级，新增根节点做等比缩放及平移。对每个 POSITION 顶点应用原节点完整世界变换，求实际包围盒，再将底部中心置于 (0, 0, 0)，高度缩放为 `heightM`。

`heightM` 是 Glowloom 构图预设的明确米制尺寸，并非作者声明的现实物品尺寸。模型文件已按这一尺寸归一，载入后不应再次按身高归一；用户仍可通过场景 transform 调整比例。

32 张 192×192 PNG 缩略图使用这些转换后的实际 GLB、Three.js 和离线 Chromium WebGL 渲染生成；并非生成式图片或替代图标。官方包的 Isometric 图像仅用于核验原资产；最终卡片使用实际交付 GLB 的渲染。

`director-model-catalog.test.ts` 逐项解析全部交付 GLB，验证实际高度、底部/中心、8 类非空、ID 唯一、缩略图存在以及未知 ID 拒绝。真实页面加载与保存由应用验收另行记录。
