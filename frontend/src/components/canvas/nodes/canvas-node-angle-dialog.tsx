import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { PreviewImage } from "@/components/canvas/runtime/components/media/preview-image";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Slider } from "@/components/ui/slider";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { RotateCcw, WandSparkles } from "lucide-react";
import { useEffect, useState } from "react";

export type CanvasImageAngleParams = {
  horizontalAngle: number;
  pitchAngle: number;
  cameraDistance: number;
  wideAngle: boolean;
};

const defaultParams: CanvasImageAngleParams = {
  horizontalAngle: 0,
  pitchAngle: 9,
  cameraDistance: 4.8,
  wideAngle: false,
};

export function CanvasNodeAngleDialog({
  dataUrl,
  open,
  onClose,
  onConfirm,
}: {
  dataUrl: string;
  open: boolean;
  onClose: () => void;
  onConfirm: (params: CanvasImageAngleParams) => void;
}) {
  const [params, setParams] = useState(defaultParams);

  useEffect(() => {
    // 选中的节点或编辑弹窗改变时重置本地草稿，避免沿用上一张素材的输入。
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (open) setParams(defaultParams);
  }, [dataUrl, open]);

  const update = <Key extends keyof CanvasImageAngleParams>(
    key: Key,
    value: CanvasImageAngleParams[Key],
  ) => setParams((current) => ({ ...current, [key]: value }));

  return (
    <Dialog
      open={open && Boolean(dataUrl)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent
        className={"dialog-canvas max-h-dialog w-full overflow-y-auto"}
      >
        <DialogHeader>
          <DialogTitle className="sr-only">调整视角</DialogTitle>
          <DialogDescription className="sr-only">
            调整图片视角参数，左侧预览方向，生成服务尚未接入。
          </DialogDescription>
        </DialogHeader>
        <div>
          <div className="flex flex-col gap-5">
            <div>
              <h2 className="text-xl font-semibold">{"AI 多角度"}</h2>
              <p className="mt-1 text-sm opacity-60">
                {"左侧只预览方向，结果会基于原图重新生成"}
              </p>
            </div>
            <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-2 2xl:grid-cols-2">
              <Card>
                <CardHeader className="sr-only">
                  <CardTitle>方向预览</CardTitle>
                </CardHeader>
                <CardContent className="flex min-h-media-preview flex-col justify-between gap-4">
                  <div className="grid flex-1 place-items-center">
                    <div className="relative">
                      <PreviewImage
                        src={dataUrl}
                        alt=""
                        className="size-48 rounded-2xl object-cover shadow-2xl"
                        draggable={false}
                        style={{ transform: previewTransform(params) }}
                      />
                      <div className="bg-media-backdrop/20 absolute -bottom-6 left-1/2 h-10 w-24 -translate-x-1/2 rounded-full border backdrop-blur" />
                    </div>
                  </div>
                  <Button
                    onClick={() => setParams(defaultParams)}
                    type={"button"}
                    variant={"secondary"}
                    size="default"
                    className={"w-fit"}
                  >
                    {<RotateCcw data-icon="inline-start" />}
                    {"重置"}
                  </Button>
                </CardContent>
              </Card>
              <div className="flex flex-col gap-6 py-2">
                <AngleSlider
                  label={"左右角度"}
                  value={params.horizontalAngle}
                  min={-60}
                  max={60}
                  step={1}
                  suffix="deg"
                  onChange={(value) => update("horizontalAngle", value)}
                />
                <AngleSlider
                  label={"俯仰角度"}
                  value={params.pitchAngle}
                  min={-45}
                  max={45}
                  step={1}
                  suffix="deg"
                  onChange={(value) => update("pitchAngle", value)}
                />
                <AngleSlider
                  label={"镜头距离"}
                  value={params.cameraDistance}
                  min={1}
                  max={10}
                  step={0.1}
                  onChange={(value) => update("cameraDistance", value)}
                />
                <div className="grid-cols-range-control grid items-center gap-4">
                  <span className="font-medium opacity-75">{"广角镜头"}</span>
                  <ToggleGroup
                    type="single"
                    variant="outline"
                    className={"w-fit"}
                    value={String(params.wideAngle ? "wide" : "standard")}
                    onValueChange={(value) => {
                      if (value)
                        ((value) => update("wideAngle", value === "wide"))(
                          value,
                        );
                    }}
                  >
                    {[
                      { label: "标准", value: "standard" },
                      { label: "广角", value: "wide" },
                    ].map((item) => {
                      const option =
                        typeof item === "object"
                          ? item
                          : { value: item, label: item };
                      return (
                        <ToggleGroupItem
                          key={String(option.value)}
                          value={String(option.value)}
                        >
                          {option.label}
                        </ToggleGroupItem>
                      );
                    })}
                  </ToggleGroup>
                </div>
              </div>
            </div>
            <div className="flex justify-end">
              <Button
                onClick={() => onConfirm(params)}
                type={"button"}
                variant={"default"}
                size="lg"
              >
                {<WandSparkles data-icon="inline-start" />}
                {"AI 生成"}
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function AngleSlider({
  label,
  value,
  min,
  max,
  step,
  suffix = "",
  onChange,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  suffix?: string;
  onChange: (value: number) => void;
}) {
  return (
    <div className="grid-cols-range-control grid items-center gap-4">
      <span className="font-medium opacity-75">{label}</span>
      <Slider
        aria-label={label}
        min={min}
        max={max}
        step={step}
        value={[value]}
        onValueChange={([value]) => onChange(value)}
      />
      <span className="text-right font-semibold whitespace-nowrap">
        {Number.isInteger(value) ? value : value.toFixed(1)}
        {suffix}
      </span>
    </div>
  );
}

function previewTransform(params: CanvasImageAngleParams) {
  const scale =
    1.08 - params.cameraDistance * 0.035 + (params.wideAngle ? -0.08 : 0);
  return `perspective(520px) rotateY(${params.horizontalAngle * -0.45}deg) rotateX(${params.pitchAngle * 0.35}deg) scale(${Math.max(0.72, Math.min(1.08, scale))})`;
}
