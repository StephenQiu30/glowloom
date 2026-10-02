"use client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { CanvasNodeData } from "../model";
import type { DirectorScene } from "./model";
import {
  createDirectorActor,
  createDirectorBillboard,
  createDirectorModel,
} from "./scene";
export function DirectorAssetControls({
  scene,
  nodes,
  disabled,
  onChange,
}: {
  scene: DirectorScene;
  nodes: CanvasNodeData[];
  disabled: boolean;
  onChange: (scene: DirectorScene) => void;
}) {
  const [selected, setSelected] = useState("");
  const assets = nodes.filter(
    (node) => node.assetId && (node.type === "model" || node.type === "image"),
  );
  const node = assets.find((item) => item.id === selected);
  const add = (actor: boolean) => {
    if (!node?.assetId || disabled || scene.objects.length >= 128) return;
    const object =
      node.type === "image"
        ? createDirectorBillboard(node.title, node.assetId, node.id)
        : actor
          ? {
              ...createDirectorActor(node.title),
              builtinActor: undefined,
              assetId: node.assetId,
              sourceNodeId: node.id,
            }
          : {
              ...createDirectorModel({
                name: node.title,
                assetId: node.assetId,
              }),
              sourceNodeId: node.id,
            };
    onChange({ ...scene, objects: [...scene.objects, object] });
  };
  return (
    <div className="space-y-2 rounded border p-2">
      <p className="text-xs text-muted-foreground">
        从本画布项目素材添加模型、演员或图片布景。
      </p>
      <Select
        value={selected}
        disabled={disabled || !assets.length}
        onValueChange={setSelected}
      >
        <SelectTrigger aria-label="导演台项目素材">
          <SelectValue placeholder="选择画布素材" />
        </SelectTrigger>
        <SelectContent>
          {assets.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.title} · {item.type === "model" ? "3D 模型" : "图片"}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          variant="outline"
          disabled={disabled || !node || scene.objects.length >= 128}
          onClick={() => add(false)}
        >
          {node?.type === "image" ? "添加布景" : "添加模型"}
        </Button>
        {node?.type === "model" ? (
          <Button
            size="sm"
            variant="outline"
            disabled={disabled || scene.objects.length >= 128}
            onClick={() => add(true)}
          >
            添加为演员
          </Button>
        ) : null}
        {node?.type === "image" ? (
          <Button
            size="sm"
            variant="outline"
            disabled={disabled}
            onClick={() =>
              onChange({
                ...scene,
                panorama: {
                  assetId: node.assetId!,
                  name: node.title,
                  rotation: 0,
                },
              })
            }
          >
            设为全景
          </Button>
        ) : null}
      </div>
      {scene.panorama ? (
        <Button
          size="sm"
          variant="ghost"
          disabled={disabled}
          onClick={() => onChange({ ...scene, panorama: undefined })}
        >
          移除全景
        </Button>
      ) : null}
      {!assets.length ? (
        <p className="text-xs text-muted-foreground">
          先在画布上传 GLB / glTF 或图片。
        </p>
      ) : null}
    </div>
  );
}
