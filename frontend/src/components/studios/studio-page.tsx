"use client";

import { useId, useState } from "react";
import dynamic from "next/dynamic";
import Image from "next/image";
import { AtSign, CircleHelp, Plus, Upload, X } from "lucide-react";
import { PageLayout } from "@/components/layout/page-layout";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { useLoginDialog } from "@/components/layout/basic-layout";
import type { SelectedSubjectMedia } from "@/components/subjects/unified-contracts";
import "./studios.css";

const SourcePicker = dynamic(() =>
  import("@/components/subjects/unified-source-picker").then(
    (module) => module.UnifiedSubjectSourcePicker,
  ),
);
const motionStyles = [
  { title: "卡片动效产品秀", image: "motion-card" },
  { title: "纸上关键词", image: "motion-paper" },
  { title: "五色孔版印刷", image: "motion-print" },
];
const marketingTemplates = [
  { title: "娃娃机好物秀", image: "marketing-left", category: "商品展示" },
  { title: "香水特写广告", image: "marketing-center", category: "创意广告" },
  { title: "鲜果环绕大片", image: "marketing-right", category: "社媒营销" },
];
const marketingTools = [
  ["视频复刻", "创意复刻，广告出彩"],
  ["商品替换", "焕新商品形象"],
  ["商品摄影", "无需棚拍，商品出片"],
  ["商品广告", "商品卖点，多样呈现"],
  ["图片多尺寸", "一张好图，多处出彩"],
  ["视频改画幅", "重构画面，智能补全"],
  ["图片翻译", "一图触达多语市场"],
  ["广告海报复刻", "复用版式，快速出新"],
  ["妆造焕新", "百变妆造，随心焕新"],
  ["图片复刻", "心仪创意，为你所用"],
];
function Cover({
  image,
  className = "",
  priority = false,
}: {
  image: string;
  className?: string;
  priority?: boolean;
}) {
  return (
    <Image
      src={`/design/reference/studios/${image}.png`}
      alt=""
      fill
      sizes="(max-width: 767px) 90vw, 370px"
      className={`object-cover ${className}`}
      priority={priority}
    />
  );
}
function Parameter({
  label,
  value,
  values,
  onChange,
}: {
  label: string;
  value: string;
  values: string[];
  onChange: (value: string) => void;
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger
        aria-label={label}
        className="w-auto border-0 bg-transparent shadow-none"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {values.map((value) => (
            <SelectItem key={value} value={value}>
              {value}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}

export function StudioPage({ kind }: { kind: "motion" | "marketing" }) {
  const session = useOptionalSession();
  const space = useSpace();
  const login = useLoginDialog();
  // Reset private selections when account or space changes.
  return (
    <StudioWorkspace
      key={`${session?.actor.id ?? "guest"}:${space.selected?.id ?? "none"}`}
      kind={kind}
      actorID={session?.actor.id}
      scopeID={space.selected?.id}
      onLogin={login}
    />
  );
}
function StudioWorkspace({
  kind,
  actorID,
  scopeID,
  onLogin,
}: {
  kind: "motion" | "marketing";
  actorID?: string;
  scopeID?: string;
  onLogin: () => void;
}) {
  const marketing = kind === "marketing";
  const promptID = useId();
  const [prompt, setPrompt] = useState("");
  const [style, setStyle] = useState("");
  const [styleOpen, setStyleOpen] = useState(false);
  const [source, setSource] = useState<string | null>(null);
  const [references, setReferences] = useState<
    Record<string, SelectedSubjectMedia>
  >({});
  const [ratio, setRatio] = useState("16:9");
  const [duration, setDuration] = useState(marketing ? "15s" : "10s");
  const [category, setCategory] = useState("全部");
  const [tool, setTool] = useState<string | null>(null);
  const styles = marketing ? marketingTemplates : motionStyles;
  function chooseSource(target: string) {
    if (!actorID) {
      onLogin();
      return;
    }
    if (scopeID) setSource(target);
  }
  return (
    <PageLayout
      screen={marketing ? "marketing-studio" : "motion-studio"}
      hideSearch
    >
      <div className="studio-workspace">
        {marketing && (
          <div
            className="studio-marketing-hero"
            role="group"
            aria-label="营销创意示例"
          >
            {marketingTemplates.map((item, index) => (
              <div
                key={item.image}
                className={
                  index === 1 ? "studio-hero-center" : "studio-hero-side"
                }
              >
                <Cover image={item.image} priority />
              </div>
            ))}
          </div>
        )}
        <h1 className="workspace-hero-title">
          {marketing
            ? "在 营销工作室 轻松做爆款"
            : "用 Motion Studio 让创意动起来"}
        </h1>
        {!marketing && (
          <p className="workspace-supporting-text text-center text-muted-foreground">
            选个风格，给点想法，让视频有声有色
          </p>
        )}
        <form
          className="studio-composer"
          onSubmit={(event) => event.preventDefault()}
        >
          <div className="studio-composer-input">
            <div className="studio-references">
              <Button
                type="button"
                variant="secondary"
                className="studio-reference"
                onClick={() => setStyleOpen(true)}
                aria-label="选择参考风格"
              >
                {style || (marketing ? "风格" : "参考风格")}
              </Button>
              {marketing &&
                ["商品", "角色"].map((target) => (
                  <Button
                    key={target}
                    type="button"
                    variant="secondary"
                    className="studio-reference"
                    onClick={() => chooseSource(target)}
                  >
                    {references[target]?.file_name || target}
                  </Button>
                ))}
            </div>
            <Label htmlFor={promptID} className="sr-only">
              提示词
            </Label>
            <Textarea
              id={promptID}
              className="studio-prompt"
              value={prompt}
              onChange={(event) => setPrompt(event.target.value)}
              placeholder={
                marketing
                  ? "请描述你想创作的营销视频"
                  : "想讲知识、秀商品、玩创意？选个风格，给点想法或素材，让视频有声有色～"
              }
            />
            <Button
              type="submit"
              className="studio-generate"
              disabled
              aria-describedby={`${promptID}-availability`}
            >
              生成
            </Button>
          </div>
          <div className="studio-parameters">
            {marketing ? (
              <>
                <span className="px-2 text-sm">视频生成</span>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => chooseSource("参考内容")}
                >
                  <Plus />
                  参考内容
                </Button>
                <Button type="button" variant="ghost" disabled>
                  暂无可用模型
                </Button>
              </>
            ) : (
              <Button
                type="button"
                variant="ghost"
                onClick={() => chooseSource("参考内容")}
              >
                <Upload />
                参考内容
              </Button>
            )}
            <Parameter
              label="画幅"
              value={ratio}
              values={["16:9", "9:16", "1:1"]}
              onChange={setRatio}
            />
            {marketing && (
              <span className="text-sm text-muted-foreground">720P</span>
            )}
            <Parameter
              label="时长"
              value={duration}
              values={["5s", "10s", "15s"]}
              onChange={setDuration}
            />
            {!marketing && (
              <>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="引用参考内容"
                  onClick={() => chooseSource("参考内容")}
                >
                  <AtSign />
                </Button>
                <span className="ml-auto text-sm">标准</span>
              </>
            )}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="生成能力说明"
                >
                  <CircleHelp />
                </Button>
              </TooltipTrigger>
              <TooltipContent>工作室生成暂未开放。</TooltipContent>
            </Tooltip>
          </div>
          {Object.entries(references).length > 0 && (
            <div
              className="flex flex-wrap gap-2"
              role="group"
              aria-label="已选参考内容"
            >
              {Object.entries(references).map(([target, media]) => (
                <Button
                  key={target}
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    setReferences((current) =>
                      Object.fromEntries(
                        Object.entries(current).filter(
                          ([key]) => key !== target,
                        ),
                      ),
                    )
                  }
                  aria-label={`移除${target} ${media.file_name}`}
                >
                  <span className="max-w-48 truncate">
                    {target} · {media.file_name}
                  </span>
                  <X />
                </Button>
              ))}
            </div>
          )}
          <p id={`${promptID}-availability`} className="sr-only" role="status">
            {marketing ? "营销" : "动效"}
            生成暂未开放。
          </p>
        </form>
        {marketing ? (
          <>
            <div
              className="studio-tool-viewport"
              role="group"
              aria-label="营销工具"
            >
              {marketingTools.map(([title, description], index) => (
                <Button
                  variant="asset-card"
                  size="asset-card"
                  key={title}
                  type="button"
                  className="studio-tool-card"
                  onClick={() => setTool(title)}
                >
                  <span className="studio-tool-cover">
                    <Cover image={marketingTemplates[index % 3].image} />
                  </span>
                  <span className="resource-card-title">{title}</span>
                  <span className="resource-card-caption text-muted-foreground">
                    {description}
                  </span>
                </Button>
              ))}
            </div>
            <ToggleGroup
              variant="workspace"
              size="toolbar"
              spacing={0}
              type="single"
              value={category}
              onValueChange={(value) => {
                if (value) setCategory(value);
              }}
              className="studio-template-categories"
              aria-label="营销模板分类"
            >
              {["全部", "社媒营销", "商品展示", "创意广告", "营销海报"].map(
                (value) => (
                  <ToggleGroupItem value={value} key={value}>
                    {value}
                  </ToggleGroupItem>
                ),
              )}
            </ToggleGroup>
          </>
        ) : (
          <h2 className="text-base font-medium">选择一个风格开始创作</h2>
        )}
        <div className="studio-template-grid">
          {styles
            .filter(
              (item) =>
                !marketing ||
                category === "全部" ||
                ("category" in item && item.category === category),
            )
            .map((item) => (
              <Button
                variant="asset-card"
                size="asset-card"
                key={item.title}
                type="button"
                className="studio-template-card"
                onClick={() => {
                  setStyle(item.title);
                  document.getElementById(promptID)?.focus();
                }}
                aria-pressed={style === item.title}
              >
                <span className="studio-template-cover">
                  <Cover image={item.image} />
                </span>
                <span className="resource-card-title">{item.title}</span>
              </Button>
            ))}
        </div>
        {marketing && category === "营销海报" && (
          <p className="py-12 text-center text-sm text-muted-foreground">
            暂无相关模板
          </p>
        )}
      </div>
      <Dialog open={styleOpen} onOpenChange={setStyleOpen}>
        <DialogContent className="sm:max-w-3xl">
          <DialogTitle>选择参考风格</DialogTitle>
          <DialogDescription>
            选择一个风格，添加到当前创作想法。
          </DialogDescription>
          <div className="studio-template-grid">
            {styles.map((item) => (
              <Button
                variant="ghost"
                type="button"
                key={item.title}
                className="studio-template-card"
                onClick={() => {
                  setStyle(item.title);
                  setStyleOpen(false);
                }}
              >
                <span className="studio-template-cover">
                  <Cover image={item.image} />
                </span>
                <span>{item.title}</span>
              </Button>
            ))}
          </div>
        </DialogContent>
      </Dialog>
      <Dialog
        open={!!tool}
        onOpenChange={(open) => {
          if (!open) setTool(null);
        }}
      >
        <DialogContent>
          <DialogTitle>{tool}</DialogTitle>
          <DialogDescription>
            此营销工具暂未开放。你可以先在工作室编辑创作想法。
          </DialogDescription>
        </DialogContent>
      </Dialog>
      {source && actorID && scopeID && (
        <SourcePicker
          actorID={actorID}
          scopeID={scopeID}
          allowedKinds={["image", "video", "audio"]}
          onClose={() => setSource(null)}
          onSelect={(media) => {
            setReferences((current) => ({ ...current, [source]: media }));
            setSource(null);
          }}
        />
      )}
    </PageLayout>
  );
}
