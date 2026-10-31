"use client";
import { useRef, useState } from "react";
import {
  Camera,
  Aperture,
  Focus,
  Circle,
  ChevronLeft,
  ChevronRight,
  Plus,
  Minus,
  Pipette,
  ImagePlus,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
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
      name: "未命名配色 1",
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
      setError(e instanceof Error ? e.message : "设置尚未保存");
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
        className="cinema-kit-dialog"
        data-canvas-no-zoom
        data-canvas-shortcuts-ignore
        onPointerDown={(e) => e.stopPropagation()}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <DialogHeader className="cinema-kit-header">
          <DialogTitle>
            {custom ? editing.name : cinemaLabels[kind]}
          </DialogTitle>
          <DialogDescription className="sr-only">
            选择视觉创作设置，确认后保存
          </DialogDescription>
          {!custom && (kind === "camera" || kind === "palette") ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() =>
                kind === "camera"
                  ? setCamera(cameraDefault())
                  : setPalette(undefined)
              }
            >
              全部重置
            </Button>
          ) : null}
        </DialogHeader>
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
                          colors: paletteHarmony(color, mode, p.colors.length),
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
                            updateColor(hsvToHex(+e.target.value, hsv.s, hsv.v))
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
          <>
            <div className="cinema-camera-preview">
              <CinemaPreview variant={catalog.camera.indexOf(camera.body)} />
            </div>
            <div className="cinema-camera-columns">
              {(
                [
                  {
                    key: "body",
                    label: "相机",
                    choices: catalog.camera,
                    Icon: Camera,
                  },
                  {
                    key: "lens",
                    label: "镜头",
                    choices: catalog.lens,
                    Icon: Focus,
                  },
                  {
                    key: "focal_length",
                    label: "焦距",
                    choices: catalog.focalLength,
                    Icon: Circle,
                  },
                  {
                    key: "aperture",
                    label: "光圈",
                    choices: catalog.aperture,
                    Icon: Aperture,
                  },
                ] as const
              ).map(({ key, label, choices, Icon }) => (
                <div key={key}>
                  <h3>{label}</h3>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`上一${label}`}
                    onClick={() =>
                      setCamera((c) => ({
                        ...c,
                        [key]:
                          choices[
                            (choices.indexOf(c[key]) + choices.length - 1) %
                              choices.length
                          ],
                      }))
                    }
                  >
                    <ChevronLeft />
                  </Button>
                  <div
                    role="radiogroup"
                    aria-label={label}
                    className="cinema-carousel"
                  >
                    {choices.map((choice) => (
                      <Button
                        variant="ghost"
                        key={choice}
                        role="radio"
                        aria-checked={camera[key] === choice}
                        onClick={() =>
                          setCamera((c) => ({ ...c, [key]: choice }))
                        }
                      >
                        <Icon />
                        <span>{choice}</span>
                      </Button>
                    ))}
                  </div>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`下一${label}`}
                    onClick={() =>
                      setCamera((c) => ({
                        ...c,
                        [key]:
                          choices[
                            (choices.indexOf(c[key]) + 1) % choices.length
                          ],
                      }))
                    }
                  >
                    <ChevronRight />
                  </Button>
                </div>
              ))}
            </div>
          </>
        ) : kind === "palette" ? (
          <>
            <div
              className="cinema-palette-grid"
              role="radiogroup"
              aria-label="配色"
            >
              {[
                ...catalog.palettes.map((p) => ({ ...p, preset: p.name })),
                ...(palette && !palette.preset ? [palette] : []),
              ].map((p) => (
                <Button
                  variant="ghost"
                  key={p.name}
                  role="radio"
                  aria-checked={palette?.name === p.name}
                  onClick={() => setPalette(p)}
                >
                  <span className="cinema-swatches">
                    {p.colors.map((c, i) => (
                      <i key={i} style={{ backgroundColor: c }} />
                    ))}
                  </span>
                  <span>{p.name}</span>
                </Button>
              ))}
            </div>
            <Popover>
              <PopoverTrigger asChild>
                <Button variant="secondary">
                  <Plus />
                  创建配色
                </Button>
              </PopoverTrigger>
              <PopoverContent data-canvas-no-zoom data-canvas-shortcuts-ignore>
                <Button variant="ghost" onClick={() => file.current?.click()}>
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
                            name: "未命名配色 1",
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
            <Input
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
                  setEditing({ name: f.name.replace(/\.[^.]+$/, ""), colors });
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
              <CinemaPreview variant={catalog.styles.indexOf(style)} />
              <h3>{style}</h3>
            </div>
            <div className="cinema-style-carousel">
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="上一风格"
                onClick={() =>
                  setStyle(
                    catalog.styles[(catalog.styles.indexOf(style) + 9) % 10],
                  )
                }
              >
                <ChevronLeft />
              </Button>
              <div role="radiogroup" aria-label="风格">
                {catalog.styles.map((s, i) => (
                  <Button
                    variant="ghost"
                    key={s}
                    role="radio"
                    aria-checked={style === s}
                    onClick={() => setStyle(s)}
                  >
                    <CinemaPreview variant={i} />
                    <span>{s}</span>
                  </Button>
                ))}
              </div>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="下一风格"
                onClick={() =>
                  setStyle(
                    catalog.styles[(catalog.styles.indexOf(style) + 1) % 10],
                  )
                }
              >
                <ChevronRight />
              </Button>
            </div>
          </>
        ) : (
          <>
            <div
              className="cinema-motion-grid"
              role="radiogroup"
              aria-label="运镜"
            >
              {catalog.motions.map((m, i) => (
                <Button
                  variant="ghost"
                  key={m}
                  role="radio"
                  aria-checked={motion === m}
                  onClick={() => setMotion(m)}
                >
                  <CinemaPreview variant={i} />
                  <span>{m}</span>
                </Button>
              ))}
            </div>
            {motion ? (
              <Button variant="ghost" onClick={() => setMotion("")}>
                清空选择
              </Button>
            ) : null}
          </>
        )}
        {error ? <p role="alert">{error}</p> : null}
        {!custom ? (
          <DialogFooter>
            <Button disabled={!valid || pending} onClick={() => void confirm()}>
              {pending ? "正在保存…" : "确认"}
            </Button>
          </DialogFooter>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
