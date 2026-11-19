"use client";

import { useId, useState } from "react";
import dynamic from "next/dynamic";
import Image from "next/image";
import Link from "next/link";
import { ArrowUpRight, AtSign, Plus, Upload, X } from "lucide-react";
import { PageLayout } from "@/components/layout/page-layout";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldLabel } from "@/components/ui/field";
import { ContentState } from "@/components/layout/content-state";
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
  { title: "产品定格", image: "cobalt-headphones" },
  { title: "幻境光效", image: "moonlight-whale" },
  { title: "山野漫游", image: "alpine-road" },
];
const marketingTemplates = [
  { title: "耳机产品展示", image: "cobalt-headphones", category: "商品展示" },
  { title: "香氛广告", image: "amber-perfume", category: "创意广告" },
  { title: "柑橘静物", image: "citrus-still-life", category: "社媒营销" },
];
const marketingTools = [
  "视频复刻",
  "商品替换",
  "商品摄影",
  "商品广告",
  "图片多尺寸",
  "视频改画幅",
  "图片翻译",
  "广告海报复刻",
  "妆造焕新",
  "图片复刻",
];
function Cover({ image }: { image: string }) {
  return (
    <Image
      src={`/design/reference/${image}.png`}
      alt=""
      fill
      loading="eager"
      sizes="(max-width: 767px) 46vw, 370px"
      className="object-cover"
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
        <header className="workspace-page-header studio-heading">
          <div className="workspace-page-heading">
            <h1>{marketing ? "营销工作室" : "动效工作室"}</h1>
          </div>
          <Button variant="secondary" asChild>
            <Link
              href={
                marketing ? "/assets?view=marketing" : "/assets?view=motion"
              }
            >
              查看资产
              <ArrowUpRight aria-hidden="true" data-icon="inline-end" />
            </Link>
          </Button>
        </header>
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
            <Field className="studio-prompt-field">
              <FieldLabel htmlFor={promptID} className="sr-only">
                提示词
              </FieldLabel>
              <Textarea
                id={promptID}
                className="studio-prompt"
                value={prompt}
                onChange={(event) => setPrompt(event.target.value)}
                placeholder={
                  marketing ? "描述想创作的营销视频" : "描述你想制作的动效"
                }
              />
            </Field>
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
              </>
            )}
            <div className="studio-submit">
              <span
                id={`${promptID}-availability`}
                className="text-sm text-muted-foreground"
              >
                生成暂未开放
              </span>
              <Button
                type="submit"
                disabled
                aria-describedby={`${promptID}-availability`}
              >
                生成
              </Button>
            </div>
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
        </form>
        <section
          aria-labelledby={`${promptID}-templates`}
          className="flex min-w-0 flex-col gap-3"
        >
          <div className="workspace-section-heading">
            <h2 id={`${promptID}-templates`}>
              {marketing ? "创作模板" : "参考风格"}
            </h2>
          </div>
          {marketing && (
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
            <ContentState title="暂无营销海报模板" className="min-h-36" />
          )}
        </section>
        {marketing && (
          <section aria-labelledby={`${promptID}-tools`}>
            <div className="workspace-section-heading">
              <h2 id={`${promptID}-tools`}>营销工具</h2>
            </div>
            <div className="flex flex-wrap gap-2">
              {marketingTools.map((title) => (
                <Button
                  key={title}
                  type="button"
                  variant="outline"
                  onClick={() => setTool(title)}
                >
                  {title}
                </Button>
              ))}
            </div>
          </section>
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
