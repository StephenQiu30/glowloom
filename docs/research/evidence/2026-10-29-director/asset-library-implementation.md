# DIR-006 / 010 / 011 / 012 资产库增量实现

审查日期：2026-10-29。工作目录为隔离实现树；本记录描述本轮文件和验证，不把并行编写中的完整导演台称为已验收。

## 交付

- `director-asset-library.tsx`：职责独立的角色、物体、背景面板，全部/我的、几何体/物体、搜索、8 类简模筛选、背景添加先选类型再选来源。
- `director-project-assets.tsx`：复用 `listMediaLibrary`、项目选择器查询与回执校验，40 项分页、去重、加载、空态、失败重试；账户/组织代次、卸载取消、素材 revision/项目/类型和父级 guard 在确认前后校验。
- `director-asset-preview.tsx` / `director-asset-preview-renderer.ts`：公开预设使用实际模型渲染 PNG；项目 GLB 通过授权预览接口取得资源，进入可视范围后按需渲染。单个短期 WebGL renderer 串行服务卡片，结束后释放；不可用时明确提供重试，不将 GLB URL 当作图片。
- `director-model-catalog.ts`：32 个白名单 `builtin_model` 注册项，导出 `DIRECTOR_MODELS` / `directorBuiltinModel(id)` / `DirectorBuiltinModel`；未知 ID 抛错。持久化与 viewport 接入由主代理及数据合同任务完成。
- `director-assets.ts`：12 个角色增加真实缩略图与实际体型高度；菜单按源端证据更新为简化人体、立方体、球体、长方体、圆柱体、圆锥体、四棱锥、圆环体、基础平面、胶囊体、圆盘、半圆弧、半圆柱。旧多面体只在数据合同兼容，不继续展示。

## 资源与许可

固定 BeefTV `2327ee2` 只有两个人体 GLB 和测试三角形，没有八类可直接取用的简模库。新增简模来自 Kenney 官方 Furniture Kit 与 Nature Kit 的公开 CC0 下载：8 类各 4 个真实 GLB，覆盖家具、建筑、自然、街道、厨房、卫浴、办公、日用。不是即梦私有目录复制，也不宣称完整收录官方资源包。

原许可、官方页面、下载包 hash、逐模型原文件/源 hash/交付 hash/实际源边界/归一缩放写入：

- `frontend/public/director/catalog/README.md`
- `frontend/public/director/catalog/{furniture-kit,nature-kit}-CC0.txt`
- `frontend/public/director/catalog/manifest.json`

交付 32 个 GLB 在源几何世界坐标中逐 POSITION 顶点测量；以额外根节点统一底部中心并缩放到清单声明的米制高度。heightM 是本项目构图预设尺度，不虚构为原作者或现实产品尺寸。资源内容不需要任意外链或新增资产上传记录。

角色 PNG 使用本项目真实男女 GLB、`reshapeDirectorQuaterniusActor` 和 `DirectorModelMotion.sample(0,false)`；几何 PNG 使用实际 `createDirectorPrimitive`；简模 PNG 使用交付的实际 GLB。全部使用离线 Chromium / Three.js WebGL 渲染，192×192。未访问本任务正式浏览器页面。角色 CC0 与体型/姿势 MIT 来源继续保留。

资源总量（文件实际字节）：catalog 665,876；actors 130,209；geometries 84,835，合计 880,920 字节。

## 组件接线合同

```ts
<DirectorAssetLibrary
  projectID={projectID}
  tab={tab} // 'actors' | 'objects' | 'backgrounds'；机位面板留在调用者
  disabled={readOnly}
  refreshKey={assetRefreshVersion} // 本地上传成功后递增，刷新项目目录
  guard={() => importBaseline({epoch, mode, projectID})}
  onActor={(id) => beginPlacement(createDirectorActor(id))}
  onPrimitive={(primitive) => create({type: 'add-object', primitive})}
  onBuiltinModel={(preset) => beginBuiltinPlacement(preset.id)}
  onLocalImport={(kind) => openLocalImport(kind)}
  onProjectPicker={(mediaKind, objectKind) => openAssetPicker(mediaKind, objectKind)}
  onProjectAssets={(assets, objectKind) => stageProjectAssets(assets, objectKind)}
  onCanvasPicker={(objectKind) => openCanvasImagePicker(objectKind)}
/>
```

上述函数名为调用者接线示意；props 类型在组件中导出。`onProjectAssets` 支持 Promise，组件已复核素材详情，调用者仍应保留本轮既有 document epoch/mode/readOnly guards 与 placement/save/History。`objectKind` 是 actor/model/billboard/panorama。平面背景走地面放置；全景直接环境 save。取消放置不删除媒体资产。组件不自行写场景或另造持久化。

“我的”明确标注“当前项目”：复用项目资产合同，不伪称账号下所有个人素材，也不把其他项目 asset_id 直接写入当前场景。更广范围资产选择仍走既有项目素材选择器。

## 验证与边界

- 新增 4 份单测共 16 项通过：真实 32 GLB 解析/实际高度/底部中心/8 类非空/ID 唯一及白名单、12 不同角色图和 13 几何图、搜索/筛选/只读、背景类型来源、真实项目查询参数、空态/重试/分页去重、素材 revision 变化、账户代次变化、卸载中止、GLB 渲染而非错误图片 URL。
- 32 个简模全部经真实离线 WebGL 渲染为对应缩略图，12 个角色与 13 个几何同样真实渲染；已人工查看标准男性、儿童及椅子图片。
- 32 个交付 GLB 的 SHA-256 与 manifest 全部一致。
- 本轮 scoped ESLint、Prettier 检查通过；完整 `tsc --noEmit --incremental false` 通过。
- DIR-012 转换/上传逻辑沿用前轮 `director-model-import.ts` 与 `director-import-dialog.tsx`：GLB/glTF/FBX/OBJ，自包含资源及 64 MiB 上限；本轮没有另起上传或导入实现。
- 本轮未直接编辑 workspace、scene、viewport、motion、backend，也未提交。真实应用账户下的资产列表、连续放置、刷新保存与画布背景引用仍由主代理在接线后做正式页面验收。
