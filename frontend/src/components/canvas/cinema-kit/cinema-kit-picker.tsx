"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { toast } from "@/components/feedback/notification";
import { useRef, useState } from "react";
import {
  ChevronLeft,
  ChevronRight,
  Plus,
  Minus,
  Pipette,
  ImagePlus,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { CinemaCameraPicker } from "./cinema-camera-picker";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from "@/components/ui/popover";
import {
  cinemaCatalog as catalog,
  cinemaLabels,
  readCinemaKit,
  cameraDefault,
  type CinemaKit,
  type CinemaKind,
  type CinemaPalette,
} from "./cinema-kit-model";
import {
  hexToHSV,
  hsvToHex,
  paletteHarmony,
  paletteFromImage,
} from "./cinema-palette";
import { CinemaPreview } from "./cinema-preview";
import "./cinema-kit.css";
export function CinemaKitPicker({
  kind,
  initial,
  onConfirm,
  onClose,
}: {
  kind: CinemaKind;
  initial?: CinemaKit;
  onConfirm: (value: CinemaKit) => Promise<void> | void;
  onClose: () => void;
}) {
  const [camera, setCamera] = useState(initial?.camera ?? cameraDefault()),
    [palette, setPalette] = useState<CinemaPalette | undefined>(
      initial?.palette,
    ),
    [style, setStyle] = useState(initial?.style ?? catalog.styles[0]),
    [motion, setMotion] = useState(initial?.motion ?? "");
  const [custom, setCustom] = useState(false),
    [editing, setEditing] = useState<CinemaPalette>({
      name: "自定义配色",
      colors: catalog.customDefault,
    }),
    [selected, setSelected] = useState(0),
    [harmony, setHarmony] = useState("自定义取色"),
    [error, setError] = useState(""),
    [pending, setPending] = useState(false);
  const file = useRef<HTMLInputElement>(null);
  const value: CinemaKit = {
    version: 1,
    ...(kind === "camera"
      ? { camera }
      : kind === "palette"
        ? { palette }
        : kind === "style"
          ? { style }
          : { motion }),
  };
  const valid =
    kind === "palette" ? !!palette : kind === "motion" ? !!motion : true;
  const color = editing.colors[selected] ?? editing.colors[0],
    hsv = hexToHSV(color);
  function updateColor(hex: string) {
    if (!/^#[\dA-F]{6}$/i.test(hex)) return;
    setEditing((p) => ({
      ...p,
      colors:
        harmony === "自定义取色"
          ? p.colors.map((c, i) => (i === selected ? hex.toUpperCase() : c))
          : paletteHarmony(hex, harmony, p.colors.length),
    }));
  }
  async function confirm() {
    setPending(true);
    setError("");
    try {
      await onConfirm(readCinemaKit(value));
      onClose();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "设置尚未保存");
    } finally {
      setPending(false);
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) {
          if (custom) setCustom(false);
          else onClose();
        }
      }}
    >
      <DialogContent
        aria-describedby={undefined}
        className="cinema-kit-dialog"
        data-cinema-kind={custom ? "custom" : kind}
        data-canvas-no-zoom
        data-canvas-shortcuts-ignore
        onPointerDown={(e) => e.stopPropagation()}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <DialogHeader className="cinema-kit-header">
          <DialogTitle>
            {custom ? editing.name : cinemaLabels[kind]}
          </DialogTitle>
        </DialogHeader>
        <div className="cinema-kit-body" inert={pending}>
          {custom ? (
            <div className="cinema-custom">
              <Input
                aria-label="配色名称"
                value={editing.name}
                maxLength={128}
                onChange={(e) =>
                  setEditing((p) => ({ ...p, name: e.target.value }))
                }
              />
              <div
                role="radiogroup"
                aria-label="取色方式"
                className="cinema-harmony"
              >
                {["自定义取色", "临近色取色", "互补色", "三等分取色"].map(
                  (mode) => (
                    <Button
                      role="radio"
                      aria-checked={harmony === mode}
                      variant={harmony === mode ? "secondary" : "ghost"}
                      key={mode}
                      onClick={() => {
                        setHarmony(mode);
                        if (mode !== "自定义取色")
                          setEditing((p) => ({
                            ...p,
                            colors: paletteHarmony(
                              color,
                              mode,
                              p.colors.length,
                            ),
                          }));
                      }}
                    >
                      {mode}
                    </Button>
                  ),
                )}
              </div>
              <Button
                variant="ghost"
                className="cinema-color-wheel"
                aria-label="色轮"
                onPointerDown={(e) => {
                  e.currentTarget.setPointerCapture(e.pointerId);
                  const r = e.currentTarget.getBoundingClientRect();
                  updateColor(
                    hsvToHex(
                      ((Math.atan2(
                        e.clientY - r.top - r.height / 2,
                        e.clientX - r.left - r.width / 2,
                      ) *
                        180) /
                        Math.PI +
                        360) %
                        360,
                      hsv.s,
                      hsv.v,
                    ),
                  );
                }}
                onPointerMove={(e) => {
                  if (!e.currentTarget.hasPointerCapture(e.pointerId)) return;
                  const r = e.currentTarget.getBoundingClientRect();
                  updateColor(
                    hsvToHex(
                      ((Math.atan2(
                        e.clientY - r.top - r.height / 2,
                        e.clientX - r.left - r.width / 2,
                      ) *
                        180) /
                        Math.PI +
                        360) %
                        360,
                      hsv.s,
                      hsv.v,
                    ),
                  );
                }}
              />
              <p>{editing.colors.length} 个颜色</p>
              <div className="cinema-color-list">
                {editing.colors.map((c, i) => (
                  <div key={i}>
                    <Button
                      variant="ghost"
                      aria-label={`颜色 ${i + 1} ${c}`}
                      aria-pressed={selected === i}
                      style={{ backgroundColor: c }}
                      onClick={() => setSelected(i)}
                    />
                    <Popover>
                      <PopoverTrigger asChild>
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label={`编辑颜色 ${i + 1}`}
                          onClick={() => setSelected(i)}
                        >
                          {c}
                        </Button>
                      </PopoverTrigger>
                      <PopoverContent
                        data-canvas-no-zoom
                        data-canvas-shortcuts-ignore
                        className="cinema-color-editor"
                      >
                        <Input
                          key={c}
                          aria-label="Hex色号"
                          defaultValue={c}
                          onBlur={(e) => {
                            if (/^#[\dA-F]{6}$/i.test(e.target.value)) {
                              updateColor(e.target.value);
                              setError("");
                            } else setError("请输入完整的六位Hex色号");
                          }}
                        />
                        <Label>
                          色相
                          <Input
                            aria-label="色相"
                            type="range"
                            min="0"
                            max="359"
                            value={hsv.h}
                            onChange={(e) =>
                              updateColor(
                                hsvToHex(+e.target.value, hsv.s, hsv.v),
                              )
                            }
                          />
                        </Label>
                        <Label>
                          饱和度
                          <Input
                            aria-label="饱和度"
                            type="range"
                            min="0"
                            max="100"
                            value={hsv.s * 100}
                            onChange={(e) =>
                              updateColor(
                                hsvToHex(hsv.h, +e.target.value / 100, hsv.v),
                              )
                            }
                          />
                        </Label>
                        <Label>
                          亮度
                          <Input
                            aria-label="亮度"
                            type="range"
                            min="0"
                            max="100"
                            value={hsv.v * 100}
                            onChange={(e) =>
                              updateColor(
                                hsvToHex(hsv.h, hsv.s, +e.target.value / 100),
                              )
                            }
                          />
                        </Label>
                        <Button
                          variant="ghost"
                          onClick={async () => {
                            const EyeDropper = (
                              window as unknown as {
                                EyeDropper?: new () => {
                                  open: () => Promise<{ sRGBHex: string }>;
                                };
                              }
                            ).EyeDropper;
                            if (!EyeDropper) {
                              setError(
                                "当前浏览器不支持屏幕取色，可输入色号或从图片取色",
                              );
                              return;
                            }
                            try {
                              updateColor(
                                (await new EyeDropper().open()).sRGBHex,
                              );
                            } catch {
                              setError("取色已取消");
                            }
                          }}
                        >
                          <Pipette />
                          屏幕取色
                        </Button>
                      </PopoverContent>
                    </Popover>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`移除颜色 ${i + 1}`}
                      disabled={editing.colors.length === 1}
                      onClick={() => {
                        setEditing((p) => ({
                          ...p,
                          colors: p.colors.filter((_, j) => j !== i),
                        }));
                        setSelected(0);
                      }}
                    >
                      <Minus />
                    </Button>
                  </div>
                ))}
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="添加颜色"
                  disabled={editing.colors.length >= 16}
                  onClick={() =>
                    setEditing((p) => ({ ...p, colors: [...p.colors, color] }))
                  }
                >
                  <Plus />
                </Button>
              </div>
              <Button
                onClick={() => {
                  try {
                    readCinemaKit({ version: 1, palette: editing });
                    setPalette(structuredClone(editing));
                    setCustom(false);
                    setError("");
                  } catch (e) {
                    setError((e as Error).message);
                  }
                }}
              >
                创建
              </Button>
            </div>
          ) : kind === "camera" ? (
            <CinemaCameraPicker value={camera} onChange={setCamera} />
          ) : kind === "palette" ? (
            <>
              <div className="cinema-palette-grid">
                <Popover>
                  <PopoverTrigger asChild>
                    <Button
                      variant="ghost"
                      size="asset-card"
                      className="cinema-create-palette"
                    >
                      <span className="cinema-create-tile">
                        <Plus />
                      </span>
                      <span>创建配色</span>
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent
                    data-canvas-no-zoom
                    data-canvas-shortcuts-ignore
                  >
                    <Button
                      variant="ghost"
                      onClick={() => file.current?.click()}
                    >
                      <ImagePlus />
                      从图片创建
                    </Button>
                    <Button
                      variant="ghost"
                      onClick={() => {
                        setEditing(
                          palette && !palette.preset
                            ? palette
                            : {
                                name: "自定义配色",
                                colors: catalog.customDefault,
                              },
                        );
                        setCustom(true);
                      }}
                    >
                      自定义配色
                    </Button>
                  </PopoverContent>
                </Popover>
                <ToggleGroup
                  type="single"
                  value={palette?.name ?? ""}
                  onValueChange={(name) =>
                    setPalette(
                      name
                        ? [
                            ...catalog.palettes.map((p) => ({
                              ...p,
                              preset: p.name,
                            })),
                            ...(palette && !palette.preset ? [palette] : []),
                          ].find((p) => p.name === name)
                        : undefined,
                    )
                  }
                  aria-label="配色"
                  className="cinema-palette-options"
                >
                  {[
                    ...catalog.palettes.map((p) => ({ ...p, preset: p.name })),
                    ...(palette && !palette.preset ? [palette] : []),
                  ].map((p) => (
                    <ToggleGroupItem
                      className="cinema-preset-option"
                      value={p.name}
                      key={p.name}
                      aria-label={p.name}
                    >
                      <span className="cinema-preset-cover">
                        <CinemaPreview
                          kind="palette"
                          name={p.name}
                          colors={p.colors}
                        />
                        <span className="cinema-swatches">
                          {p.colors.map((c, i) => (
                            <i key={i} style={{ backgroundColor: c }} />
                          ))}
                        </span>
                      </span>
                      <span className="cinema-preset-label">{p.name}</span>
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              </div>
              <Input
                hidden
                ref={file}
                type="file"
                accept="image/*"
                className="sr-only"
                aria-label="从图片创建配色"
                onChange={async (e) => {
                  const f = e.target.files?.[0];
                  if (!f) return;
                  try {
                    const colors = await paletteFromImage(f);
                    setEditing({
                      name: f.name.replace(/\.[^.]+$/, ""),
                      colors,
                    });
                    setSelected(0);
                    setCustom(true);
                  } catch (error) {
                    setError((error as Error).message);
                  } finally {
                    e.target.value = "";
                  }
                }}
              />
            </>
          ) : kind === "style" ? (
            <>
              <div className="cinema-style-preview">
                <div className="cinema-style-stage">
                  {[-1, 0, 1].map((offset) => {
                    const name =
                      catalog.styles[
                        (catalog.styles.indexOf(style) +
                          offset +
                          catalog.styles.length) %
                          catalog.styles.length
                      ];
                    return (
                      <div
                        key={offset}
                        data-position={offset}
                        aria-hidden={offset !== 0}
                      >
                        <CinemaPreview kind="style" name={name} />
                      </div>
                    );
                  })}
                </div>
                <h3>{style}</h3>
              </div>
              <div className="cinema-style-carousel">
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="上一风格"
                  onClick={() =>
                    setStyle(
                      catalog.styles[
                        (catalog.styles.indexOf(style) +
                          catalog.styles.length -
                          1) %
                          catalog.styles.length
                      ],
                    )
                  }
                >
                  <ChevronLeft />
                </Button>
                <ToggleGroup
                  type="single"
                  value={style}
                  onValueChange={(value) => {
                    if (value) setStyle(value);
                  }}
                  aria-label="风格"
                >
                  {catalog.styles.map((s) => (
                    <ToggleGroupItem value={s} key={s} aria-label={s}>
                      <span aria-hidden="true" className="cinema-style-dot" />
                      <span className="sr-only">{s}</span>
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="下一风格"
                  onClick={() =>
                    setStyle(
                      catalog.styles[
                        (catalog.styles.indexOf(style) + 1) %
                          catalog.styles.length
                      ],
                    )
                  }
                >
                  <ChevronRight />
                </Button>
              </div>
            </>
          ) : (
            <>
              <ToggleGroup
                type="single"
                value={motion}
                onValueChange={setMotion}
                className="cinema-motion-grid"
                aria-label="运镜"
              >
                {catalog.motions.map((m) => (
                  <ToggleGroupItem
                    className="cinema-preset-option"
                    value={m}
                    key={m}
                    aria-label={m}
                  >
                    <span className="cinema-preset-cover">
                      <CinemaPreview kind="motion" name={m} />
                    </span>
                    <span className="cinema-preset-label">{m}</span>
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </>
          )}
        </div>
        {error ? <FeedbackToast message={error} /> : null}
        {!custom ? (
          <DialogFooter>
            <Button
              className="min-w-20"
              disabled={!valid || pending}
              onClick={() => void confirm()}
            >
              {pending ? "正在保存…" : "确认"}
            </Button>
          </DialogFooter>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
