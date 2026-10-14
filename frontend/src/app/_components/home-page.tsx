"use client";
import { useState, useRef } from "react";
import Link from "next/link";
import {
  Sparkles,
  Plus,
  ArrowUp,
  AtSign,
  SlidersHorizontal,
  ChevronRight,
  AudioLines,
  ImageIcon,
  VideoIcon,
  X,
  Search,
} from "lucide-react";
import {
  ProductShell,
  useLoginDialog,
} from "@/components/layout/product-shell";
import { useOptionalSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Field, FieldGroup } from "@/components/ui/field";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  InputGroupTextarea,
} from "@/components/ui/input-group";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
} from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  GenerationDialog,
  type GenerationSeed,
} from "@/components/generation/generation-dialog";
import { InspirationGallery } from "@/components/inspiration/gallery";
import { UnavailableDialog } from "@/components/availability/unavailable";
const skills = [
  {
    name: "电影级长镜头运镜",
    text: "电影长镜头，平稳推进，连续光影与空间关系，保持主体一致。",
    description: "用连续运镜组织空间与光影。",
    badge: "New",
  },
  {
    name: "创作分镜",
    text: "分镜构图，明确景别、主体动作、环境与镜头方向。",
    description: "将创意转化为电影级美学视频分镜 Prompt。",
    badge: "New",
  },
  {
    name: "风格大师",
    text: "统一色彩与材质风格，明确光线方向，突出主体。",
    description: "统一画面的色彩、材质与光线。",
    badge: "New",
  },
  {
    name: "叙事短片导演分镜",
    text: "用画面呈现事件起因与情绪变化，保持人物和场景连续。",
    description: "从脚本开始组织连贯的镜头叙事。",
    badge: "Hot",
  },
  {
    name: "微表情导演",
    text: "人物近景，细腻自然的微表情，克制的动作与情绪。",
    description: "用近景捕捉自然的微表情。",
    badge: "Hot",
  },
];
function HomeContent() {
  const session = useOptionalSession();
  const login = useLoginDialog();
  const [mode, setMode] = useState("generate");
  const [kind, setKind] = useState("agent");
  const [prompt, setPrompt] = useState("");
  const [generation, setGeneration] = useState<GenerationSeed | null>(null);
  const [presets, setPresets] = useState(false);
  const [query, setQuery] = useState("");
  const [skill, setSkill] = useState<string | null>(null);
  const [feature, setFeature] = useState<string | null>(null);
  const input = useRef<HTMLTextAreaElement>(null);
  function openGeneration(pickSubject = false) {
    if (!session) {
      login();
      return;
    }
    if (kind === "agent") {
      setFeature("Agent 创作");
      return;
    }
    setGeneration({
      prompt,
      capability: kind === "video" ? "video" : "image",
      pickSubject,
    });
  }
  function chooseSkill(name: string) {
    const selected = skills.find((s) => s.name === name);
    if (selected) {
      setSkill(name);
      setPrompt((p) => selected.text + (p ? "\n" + p : ""));
    }
    setPresets(false);
    input.current?.focus();
  }
  return (
    <div className="home-page">
      <section className="creation-hero" aria-label="开始创作">
        <h1>你好，今天想要创作什么？</h1>
        <ToggleGroup
          type="single"
          variant="pill"
          size="hero"
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
              onSubmit={(e) => {
                e.preventDefault();
                if (prompt.trim()) openGeneration();
              }}
            >
              <FieldGroup>
                <Field>
                  <InputGroup variant="composer" className="creation-composer">
                    <div className="creation-input">
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            type="button"
                            variant="reference"
                            size="reference"
                            className="reference-add"
                            aria-label="上传参考"
                            onClick={() => openGeneration()}
                          >
                            <Plus />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>添加参考素材</TooltipContent>
                      </Tooltip>
                      <div className="creation-text">
                        {skill && (
                          <Badge variant="secondary">
                            {skill}
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon-xs"
                              aria-label="移除技能"
                              onClick={() => setSkill(null)}
                            >
                              <X />
                            </Button>
                          </Badge>
                        )}
                        <InputGroupTextarea
                          ref={input}
                          value={prompt}
                          maxLength={10000}
                          onChange={(e) => setPrompt(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === "/" && !prompt) {
                              e.preventDefault();
                              setPresets(true);
                            }
                          }}
                          aria-label="创作提示词"
                          placeholder="输入想法、剧本或上传参考，支持 / 使用技能，@ 添加主体，和 Agent 一起创作"
                        />
                      </div>
                    </div>
                    <InputGroupAddon
                      align="block-end"
                      className="creation-tools"
                    >
                      <Select value={kind} onValueChange={setKind}>
                        <SelectTrigger variant="quiet" aria-label="生成类型">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectGroup>
                            <SelectItem value="agent">
                              <Sparkles data-icon="inline-start" />
                              Agent 模式
                            </SelectItem>
                            <SelectItem value="image">
                              <ImageIcon data-icon="inline-start" />
                              图片生成
                            </SelectItem>
                            <SelectItem value="video">
                              <VideoIcon data-icon="inline-start" />
                              视频生成
                            </SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <Button
                        type="button"
                        variant="ghost"
                        onClick={() => openGeneration()}
                      >
                        <SlidersHorizontal data-icon="inline-start" />
                        {kind === "agent" ? "自动" : "参数"}
                      </Button>
                      <Popover open={presets} onOpenChange={setPresets}>
                        <PopoverTrigger asChild>
                          <Button type="button" variant="ghost">
                            <Sparkles data-icon="inline-start" />
                            技能
                          </Button>
                        </PopoverTrigger>
                        <PopoverContent
                          align="start"
                          side="bottom"
                          className="skill-popover"
                        >
                          <InputGroup variant="quiet">
                            <InputGroupAddon>
                              <Search />
                            </InputGroupAddon>
                            <InputGroupInput
                              aria-label="搜索技能"
                              placeholder="搜索技能"
                              value={query}
                              onChange={(e) => setQuery(e.target.value)}
                            />
                          </InputGroup>
                          <div className="skill-list">
                            {skills
                              .filter((s) => s.name.includes(query.trim()))
                              .map((s) => (
                                <Button
                                  key={s.name}
                                  variant="ghost"
                                  className="skill-option"
                                  onClick={() => chooseSkill(s.name)}
                                >
                                  <span>
                                    <Sparkles data-icon="inline-start" />
                                    {s.name}
                                    <Badge variant="secondary">提示</Badge>
                                  </span>
                                  <small>{s.description}</small>
                                </Button>
                              ))}
                            {!skills.some((s) =>
                              s.name.includes(query.trim()),
                            ) && (
                              <p className="p-4 text-sm text-muted-foreground">
                                没有匹配的提示技能
                              </p>
                            )}
                          </div>
                        </PopoverContent>
                      </Popover>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            className="subject-add"
                            aria-label="添加主体"
                            onClick={() => openGeneration(true)}
                          >
                            <AtSign />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>添加主体</TooltipContent>
                      </Tooltip>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            className="voice-input ml-auto"
                            aria-label="语音输入"
                            onClick={() => setFeature("语音输入")}
                          >
                            <AudioLines />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>语音输入</TooltipContent>
                      </Tooltip>
                      <Button
                        type="submit"
                        size="icon-lg"
                        className="send-prompt"
                        disabled={!prompt.trim()}
                        aria-label="开始生成"
                      >
                        <ArrowUp />
                      </Button>
                    </InputGroupAddon>
                  </InputGroup>
                </Field>
              </FieldGroup>
            </form>
            <div className="skill-chips" aria-label="推荐提示技能">
              {skills.map((s) => (
                <Button
                  key={s.name}
                  variant="secondary"
                  size="skill"
                  onClick={() => chooseSkill(s.name)}
                >
                  / {s.name}
                  <Badge variant={s.badge === "New" ? "new" : "hot"}>
                    {s.badge}
                  </Badge>
                </Button>
              ))}
              <Button
                variant="secondary"
                size="skill"
                onClick={() => setPresets(true)}
              >
                更多技能
                <ChevronRight data-icon="inline-end" />
              </Button>
            </div>
          </>
        ) : (
          <div className="home-canvas-launch">
            <div className="home-canvas-list">
              <Button variant="tile" asChild>
                <Link href="/projects">
                  <Plus />
                  <span>新建画布</span>
                </Link>
              </Button>
              <Button variant="tile" asChild>
                <Link href="/projects">
                  <span>我的画布</span>
                  <small>查看已保存的创作</small>
                </Link>
              </Button>
            </div>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                if (!session) login();
                else setFeature("从提示词创建画布");
              }}
            >
              <InputGroup variant="composer" className="canvas-idea">
                <InputGroupInput
                  aria-label="画布创作想法"
                  placeholder="输入创作想法，开始一张新画布"
                  value={prompt}
                  onChange={(e) => setPrompt(e.target.value)}
                />
                <InputGroupAddon align="inline-end">
                  <Button
                    type="submit"
                    size="icon"
                    disabled={!prompt.trim()}
                    aria-label="从想法创建画布"
                  >
                    <ArrowUp />
                  </Button>
                </InputGroupAddon>
              </InputGroup>
            </form>
          </div>
        )}
      </section>
      <InspirationGallery />
      {generation && (
        <GenerationDialog
          seed={generation}
          onClose={() => setGeneration(null)}
        />
      )}
      <UnavailableDialog feature={feature} onClose={() => setFeature(null)} />
    </div>
  );
}
export function HomePage() {
  return (
    <ProductShell screen="home" fullBleed allowGuest>
      <HomeContent />
    </ProductShell>
  );
}
