"use client";
import { useState } from "react";
import Link from "next/link";
import {
  Sparkles,
  Plus,
  ArrowUp,
  AtSign,
  SlidersHorizontal,
  ChevronRight,
  Grid2X2,
} from "lucide-react";
import { ProductShell } from "@/components/layout/product-shell";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  GenerationDialog,
  type GenerationSeed,
} from "@/components/generation/generation-dialog";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { InspirationGallery } from "@/components/inspiration/gallery";
const skills = [
  "电影级长镜头运镜",
  "创作分镜",
  "风格大师",
  "叙事短片导演分镜",
  "微表情导演",
];
const presetText: Record<string, string> = {
  电影级长镜头运镜: "电影长镜头，平稳推进，连续光影与空间关系，保持主体一致。",
  创作分镜: "分镜构图，明确景别、主体动作、环境与镜头方向。",
  风格大师: "统一色彩与材质风格，明确光线方向，突出主体。",
  叙事短片导演分镜: "用画面呈现事件起因与情绪变化，保持人物和场景连续。",
  微表情导演: "人物近景，细腻自然的微表情，克制的动作与情绪。",
};
export function HomePage() {
  const [mode, setMode] = useState("generate");
  const [kind, setKind] = useState("图片生成");
  const [prompt, setPrompt] = useState("");
  const [generation, setGeneration] = useState<GenerationSeed | null>(null);
  const [presets, setPresets] = useState(false);
  const openGeneration = (pickSubject = false) =>
    setGeneration({
      prompt,
      capability: kind === "视频生成" ? "video" : "image",
      pickSubject,
    });
  return (
    <ProductShell screen="home" fullBleed>
      <div className="home-page">
        <section className="creation-hero">
          <h1>你好，今天想要创作什么？</h1>
          <ToggleGroup
            type="single"
            value={mode}
            onValueChange={(v) => {
              if (v) setMode(v);
            }}
            className="creation-toggle"
            aria-label="创作模式"
          >
            <ToggleGroupItem value="generate">生成</ToggleGroupItem>
            <ToggleGroupItem value="canvas">画布</ToggleGroupItem>
          </ToggleGroup>
          {mode === "generate" ? (
            <>
              <form
                className="creation-composer"
                onSubmit={(e) => {
                  e.preventDefault();
                  openGeneration();
                }}
              >
                <div className="creation-input">
                  <Button
                    type="button"
                    variant="ghost"
                    aria-label="上传参考"
                    className="reference-add"
                    onClick={() => openGeneration()}
                  >
                    <Plus />
                  </Button>
                  <Textarea
                    value={prompt}
                    maxLength={10000}
                    onChange={(e) => setPrompt(e.target.value)}
                    aria-label="创作提示词"
                    placeholder="输入想法或描述画面，添加参考素材与主体，开始创作"
                  />
                </div>
                <div className="creation-tools">
                  <Select value={kind} onValueChange={setKind}>
                    <SelectTrigger
                      className="mode-select"
                      aria-label="生成类型"
                    >
                      <Sparkles size={16} />
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {["图片生成", "视频生成"].map((k) => (
                        <SelectItem key={k} value={k}>
                          {k}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => openGeneration()}
                  >
                    <SlidersHorizontal />
                    参数
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => setPresets(true)}
                  >
                    <Sparkles />
                    灵感提示
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label="添加主体"
                    onClick={() => openGeneration(true)}
                  >
                    <AtSign />
                  </Button>
                  <Button
                    type="submit"
                    size="icon-round"
                    className="ml-auto"
                    disabled={!prompt.trim()}
                    aria-label="开始生成"
                  >
                    <ArrowUp />
                  </Button>
                </div>
              </form>
              <div className="skill-chips">
                {skills.map((s, i) => (
                  <Button
                    key={s}
                    variant="outline"
                    onClick={() => {
                      setPrompt((p) => `${presetText[s]}\n${p}`);
                    }}
                  >
                    / {s}
                    <span className={i > 2 ? "hot" : "new"}>
                      {i > 2 ? "Hot" : "New"}
                    </span>
                  </Button>
                ))}
                <Button variant="outline" onClick={() => setPresets(true)}>
                  更多提示
                  <ChevronRight />
                </Button>
              </div>
            </>
          ) : (
            <div className="canvas-launch">
              <Grid2X2 size={32} strokeWidth={1.2} />
              <h2>让灵感在画布上展开</h2>
              <p>从一张空白画布开始，把图片和视频草稿组织成故事。</p>
              <Button asChild>
                <Link href="/projects">
                  打开我的画布
                  <ArrowUp />
                </Link>
              </Button>
            </div>
          )}
        </section>
        <InspirationGallery />
      </div>
      {generation && (
        <GenerationDialog
          seed={generation}
          onClose={() => setGeneration(null)}
        />
      )}
      <Dialog open={presets} onOpenChange={setPresets}>
        <DialogContent>
          <DialogTitle>创作灵感提示</DialogTitle>
          <DialogDescription>
            选择一个方向，将提示加入当前输入后继续完善。
          </DialogDescription>
          {skills.map((s) => (
            <Button
              key={s}
              variant="outline"
              onClick={() => {
                setPrompt((p) => `${presetText[s]}\n${p}`);
                setPresets(false);
              }}
            >
              {s}
            </Button>
          ))}
        </DialogContent>
      </Dialog>
    </ProductShell>
  );
}
