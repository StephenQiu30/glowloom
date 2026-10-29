"use client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { DIRECTOR_ACTORS, DIRECTOR_GEOMETRIES } from "./director-assets";
import {
  DIRECTOR_MODELS,
  DIRECTOR_MODEL_CATEGORIES,
  type DirectorBuiltinModel,
} from "./director-model-catalog";
import { DirectorAssetPreview } from "./director-asset-preview";
import {
  DirectorProjectAssets,
  type DirectorAssetObjectKind,
} from "./director-project-assets";
import "./director-asset-library.css";

export type DirectorAssetLibraryProps = {
  projectID: string;
  tab: "actors" | "objects" | "backgrounds";
  disabled?: boolean;
  refreshKey?: number;
  guard: () => void;
  onActor: (id: string) => void;
  onPrimitive: (primitive: string) => void;
  onBuiltinModel: (model: DirectorBuiltinModel) => void;
  onLocalImport: (kind: "actor" | "model") => void;
  onProjectPicker: (
    kind: "model" | "image",
    objectKind: DirectorAssetObjectKind,
  ) => void;
  onProjectAssets: (
    assets: API.AssetSummary[],
    kind: DirectorAssetObjectKind,
  ) => Promise<void> | void;
  onCanvasPicker: (kind: "billboard" | "panorama") => void;
};
export function DirectorAssetLibrary(props: DirectorAssetLibraryProps) {
  return <AssetLibrary key={`${props.projectID}/${props.tab}`} {...props} />;
}
function AssetLibrary(props: DirectorAssetLibraryProps) {
  const { tab, disabled, projectID } = props;
  const [search, setSearch] = useState(""),
    [filter, setFilter] = useState("all"),
    [category, setCategory] = useState("all"),
    [background, setBackground] = useState<"billboard" | "panorama">(
      "billboard",
    ),
    [addBackground, setAddBackground] = useState(false);
  const label = tab === "actors" ? "角色" : tab === "objects" ? "物体" : "背景";
  const query = search.trim().toLocaleLowerCase();
  const matches = (name: string) => name.toLocaleLowerCase().includes(query);
  const actors = DIRECTOR_ACTORS.filter((actor) => matches(actor.name));
  const geometries = DIRECTOR_GEOMETRIES.filter(([, name]) => matches(name));
  const models = DIRECTOR_MODELS.filter(
    (model) =>
      (category === "all" || model.category === category) &&
      matches(`${model.name} ${model.categoryLabel}`),
  );
  const objectKind =
    tab === "actors" ? "actor" : tab === "objects" ? "model" : background;
  const showMine = filter === "all" || filter === "mine";
  return (
    <div className="director-asset-library">
      <Input
        aria-label={`搜索素材 · ${label}`}
        placeholder="搜索素材"
        value={search}
        onChange={(event) => setSearch(event.target.value)}
      />
      <Tabs value={filter} onValueChange={setFilter}>
        <TabsList aria-label={`${label}来源`} className="w-full">
          <TabsTrigger value="all">全部</TabsTrigger>
          {tab === "objects" && (
            <>
              <TabsTrigger value="geometry">几何体</TabsTrigger>
              <TabsTrigger value="models">物体</TabsTrigger>
            </>
          )}
          <TabsTrigger value="mine">我的</TabsTrigger>
        </TabsList>
      </Tabs>
      <div className="director-library-actions">
        {tab !== "backgrounds" ? (
          <>
            <Button
              variant="outline"
              disabled={disabled}
              onClick={() =>
                props.onLocalImport(tab === "actors" ? "actor" : "model")
              }
            >
              导入本地{label}
            </Button>
            <Button
              variant="ghost"
              disabled={disabled}
              onClick={() => props.onProjectPicker("model", objectKind)}
            >
              从项目选择{label}
            </Button>
          </>
        ) : (
          <>
            <Button
              variant="outline"
              disabled={disabled}
              onClick={() => setAddBackground(true)}
            >
              添加背景
            </Button>
            <Tabs
              value={background}
              onValueChange={(value) =>
                setBackground(value as "billboard" | "panorama")
              }
            >
              <TabsList aria-label="背景类型">
                <TabsTrigger value="panorama">全景背景</TabsTrigger>
                <TabsTrigger value="billboard">平面背景</TabsTrigger>
              </TabsList>
            </Tabs>
          </>
        )}
      </div>
      {showMine && (
        <DirectorProjectAssets
          key={props.refreshKey ?? 0}
          projectID={projectID}
          kind={tab === "backgrounds" ? "image" : "model"}
          objectKind={objectKind}
          search={search}
          disabled={disabled}
          guard={props.guard}
          onSelect={props.onProjectAssets}
        />
      )}
      {tab === "actors" && filter === "all" && (
        <section aria-label="预设角色">
          <h4>预设</h4>
          {!actors.length && (
            <p className="director-library-empty">没有匹配的预设角色</p>
          )}
          <div className="director-library-grid">
            {actors.map((actor) => (
              <Button
                key={actor.id}
                className="director-library-card"
                variant="ghost"
                disabled={disabled}
                onClick={() => props.onActor(actor.id)}
              >
                <DirectorAssetPreview
                  thumbnail={actor.thumbnail}
                  name={actor.name}
                />
                <span className="director-library-card-label">
                  <span>{actor.name}</span>
                  <small>预设 · 高 {actor.heightM.toFixed(2)} m</small>
                </span>
              </Button>
            ))}
          </div>
        </section>
      )}
      {tab === "objects" && (filter === "all" || filter === "geometry") && (
        <section aria-label="几何体">
          <h4>几何体</h4>
          {!geometries.length && (
            <p className="director-library-empty">没有匹配的几何体</p>
          )}
          <div className="director-library-grid">
            {geometries.map(([id, name]) => (
              <Button
                key={id}
                className="director-library-card"
                variant="ghost"
                disabled={disabled}
                onClick={() => props.onPrimitive(id)}
              >
                <DirectorAssetPreview
                  thumbnail={`/director/geometries/${id}.png`}
                  name={name}
                />
                <span className="director-library-card-label">
                  <span>{name}</span>
                  <small>几何体</small>
                </span>
              </Button>
            ))}
          </div>
        </section>
      )}
      {tab === "objects" && (filter === "all" || filter === "models") && (
        <section aria-label="简模物体">
          <h4>简模物体</h4>
          <Select value={category} onValueChange={setCategory}>
            <SelectTrigger aria-label="简模分类">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部分类</SelectItem>
              {DIRECTOR_MODEL_CATEGORIES.map(([id, label]) => (
                <SelectItem key={id} value={id}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {!models.length && (
            <p className="director-library-empty">没有匹配的简模物体</p>
          )}
          <div className="director-library-grid">
            {models.map((model) => (
              <Button
                key={model.id}
                className="director-library-card"
                variant="ghost"
                disabled={disabled}
                onClick={() => props.onBuiltinModel(model)}
              >
                <DirectorAssetPreview
                  thumbnail={model.thumbnail}
                  name={model.name}
                />
                <span className="director-library-card-label">
                  <span>{model.name}</span>
                  <small>
                    {model.categoryLabel} · 高 {model.heightM} m
                  </small>
                </span>
              </Button>
            ))}
          </div>
        </section>
      )}
      {addBackground && (
        <Dialog open onOpenChange={setAddBackground}>
          <DialogContent>
            <DialogTitle>添加背景</DialogTitle>
            <DialogDescription>
              先选择背景类型，再选择图片来源。
            </DialogDescription>
            <Tabs
              value={background}
              onValueChange={(value) =>
                setBackground(value as "billboard" | "panorama")
              }
            >
              <TabsList aria-label="添加背景类型">
                <TabsTrigger value="panorama">全景背景</TabsTrigger>
                <TabsTrigger value="billboard">平面背景</TabsTrigger>
              </TabsList>
            </Tabs>
            <p>
              {background === "panorama"
                ? "全景图片环绕场景，使用完整的等距柱状投影图片。"
                : "平面图片作为布景，选定后点击地面放置。"}
            </p>
            <Button
              disabled={disabled}
              onClick={() => {
                setAddBackground(false);
                props.onProjectPicker("image", background);
              }}
            >
              从项目选择
            </Button>
            <Button
              variant="outline"
              disabled={disabled}
              onClick={() => {
                setAddBackground(false);
                props.onCanvasPicker(background);
              }}
            >
              从画布选择
            </Button>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}
