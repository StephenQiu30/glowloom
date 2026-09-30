"use client";

import Image from "next/image";
import Link from "next/link";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { shots, episodes } from "./data";
import {
  PageHeading,
  Panel,
  DataTable,
  EmptyMessage,
  DraftNotice,
  ChoiceField,
  QuotePreview,
  usePreviewQuery,
} from "./workbench-components";

export function ShotsPage({
  projectId,
  episodeId,
  storyboard = false,
}: {
  projectId: string;
  episodeId: string;
  storyboard?: boolean;
}) {
  const { params, set, readOnly } = usePreviewQuery();
  const q = params.get("q") ?? "";
  const list = shots.filter((s) =>
    `${s.title}${s.scene}${s.character}${s.status}`.includes(q),
  );
  const root = `/projects/${projectId}/episodes/${episodeId}`;
  return (
    <div className="space-y-8">
      <PageHeading
        title={storyboard ? "镜头之间，故事成形。" : "把场景拆成镜头。"}
        description={`${episodes.find((e) => e.id === episodeId)?.name} · 镜头描述、引用与时长共同构成生成输入。`}
        action={
          <QuotePreview
            target="第一集 · 镜头批次"
            label="预览批量报价"
            disabled={readOnly}
          />
        }
      />
      <div className="flex flex-wrap items-center justify-between gap-4">
        <Input
          aria-label="搜索镜头"
          placeholder="搜索镜头、场景或状态…"
          value={q}
          onChange={(e) => set("q", e.target.value)}
          className="sm:max-w-xs"
        />
        <Button variant="secondary" asChild>
          <Link href={`${root}/${storyboard ? "shots" : "storyboard"}`}>
            {storyboard ? "分镜列表" : "故事板视图"}
          </Link>
        </Button>
      </div>
      {list.length === 0 ? (
        <EmptyMessage title="没有匹配的镜头" />
      ) : storyboard ? (
        <div className="grid gap-7 md:grid-cols-2 xl:grid-cols-3">
          {list.map((s) => (
            <Panel
              key={s.id}
              title={`${s.number} · ${s.title}`}
              description={`${s.camera} / ${s.movement} / ${s.duration}s`}
            >
              <Link
                href={`${root}/shots/${s.id}`}
                className="block rounded-lg focus-visible:ring-2 focus-visible:ring-blue-500"
              >
                <Image
                  src="/examples/media/scene.svg"
                  width={560}
                  height={340}
                  alt={`${s.title}的内置样例缩略图`}
                  className="aspect-video w-full rounded-lg object-cover"
                />
                <p className="mt-4 text-sm leading-7 text-muted-foreground">
                  {s.prompt}
                </p>
              </Link>
              <div className="mt-4 flex items-center justify-between">
                <Badge
                  variant={
                    s.status === "生成失败" ? "destructive" : "secondary"
                  }
                >
                  {s.status}
                </Badge>
                <span className="text-xs text-muted-foreground">
                  {s.id === "shot-01" ? "样例选定：候选 A" : "尚未选定"}
                </span>
              </div>
            </Panel>
          ))}
        </div>
      ) : (
        <DataTable
          caption="分镜列表"
          columns={["镜头", "场景 / 角色", "景别 / 运镜", "时长", "状态"]}
          rows={list.map((s) => [
            <Link
              key={s.id}
              href={`${root}/shots/${s.id}`}
              className="font-medium hover:underline"
            >
              {s.number} · {s.title}
            </Link>,
            `${s.scene} / ${s.character}`,
            `${s.camera} / ${s.movement}`,
            `${s.duration}s`,
            <Badge
              key="state"
              variant={s.status === "生成失败" ? "destructive" : "secondary"}
            >
              {s.status}
            </Badge>,
          ])}
        />
      )}
    </div>
  );
}
export function ShotDetailPage({
  projectId,
  episodeId,
  shotId,
}: {
  projectId: string;
  episodeId: string;
  shotId: string;
}) {
  const shot = shots.find((s) => s.id === shotId)!;
  const { params, set, readOnly } = usePreviewQuery();
  const [prompt, setPrompt] = useState<string>(shot.prompt);
  const [duration, setDuration] = useState(String(shot.duration));
  const [camera, setCamera] = useState<string>(shot.camera);
  const [impact, setImpact] = useState(false);
  const [proposed, setProposed] = useState<string | null>(null);
  const candidate = params.get("candidate") === "b" ? "B" : "A";
  return (
    <div className="space-y-8">
      <PageHeading
        eyebrow={`EPISODE / SHOT ${shot.number}`}
        title={shot.title}
        description="从画面描述到关键帧和视频。生成、候选比较与正式选定是不同的操作。"
        action={
          <QuotePreview
            disabled={readOnly}
            target={`镜头 ${shot.number} · 视频`}
          />
        }
      />
      <div className="grid gap-7 xl:grid-cols-[minmax(210px,0.8fr)_minmax(280px,1.5fr)]">
        <Panel title="镜头参数" description="样例模型 · 结构化引用最多 3 项">
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="shot-prompt">画面描述</FieldLabel>
              <Textarea
                id="shot-prompt"
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                disabled={readOnly}
                className="min-h-36 leading-7"
              />
            </Field>
            <ChoiceField
              id="shot-camera"
              label="景别"
              value={camera}
              options={["远景", "中景", "特写"]}
              onChange={setCamera}
              disabled={readOnly}
            />
            <ChoiceField
              id="shot-duration"
              label="时长（秒）"
              value={duration}
              options={["4", "5", "6"]}
              onChange={setDuration}
              disabled={readOnly}
            />
            <DraftNotice
              changed={
                prompt !== shot.prompt ||
                duration !== String(shot.duration) ||
                camera !== shot.camera
              }
            />
            <div>
              <p className="mb-3 text-sm font-medium">结构化引用 · 2 / 3</p>
              <div className="flex flex-wrap gap-2">
                <Badge variant="secondary">{shot.character} / v3</Badge>
                <Badge variant="secondary">{shot.scene} / v2</Badge>
              </div>
            </div>
            <Button disabled>保存服务待接入</Button>
          </FieldGroup>
        </Panel>
        <div className="space-y-6">
          <Panel
            title="候选比较"
            description="所有图片 / 视频为内置样例；候选按钮会保留在链接中。"
          >
            <Tabs defaultValue="image">
              <TabsList variant="line">
                <TabsTrigger value="image">关键帧</TabsTrigger>
                <TabsTrigger value="video">视频</TabsTrigger>
              </TabsList>
              <TabsContent value="image">
                <Image
                  src="/examples/media/scene.svg"
                  width={560}
                  height={340}
                  alt={`候选 ${candidate} · 内置山景插画，非真实镜头生成结果`}
                  className={`mt-4 aspect-video w-full rounded-lg object-cover ${candidate === "B" ? "grayscale" : ""}`}
                />
              </TabsContent>
              <TabsContent value="video">
                <video
                  src="/examples/media/preview.mp4"
                  poster="/examples/media/scene.svg"
                  preload="none"
                  controls
                  aria-label="镜头视频样例"
                  className="mt-4 aspect-video w-full rounded-lg"
                />
              </TabsContent>
            </Tabs>
            <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
              <div role="group" aria-label="候选比较" className="flex gap-2">
                {["A", "B"].map((c) => (
                  <Button
                    key={c}
                    variant={candidate === c ? "secondary" : "ghost"}
                    aria-pressed={candidate === c}
                    onClick={() => set("candidate", c.toLowerCase())}
                  >
                    候选 {c}
                  </Button>
                ))}
              </div>
              <Button
                variant="outline"
                disabled={readOnly}
                onClick={() => setImpact(true)}
              >
                预览选定影响
              </Button>
            </div>
            <p className="mt-3 text-xs text-muted-foreground">
              样例已选定：候选 A · 当前比较：候选 {candidate}
            </p>
            {proposed && (
              <p role="status" className="mt-3 text-sm">
                本页拟选定：候选 {proposed} · 未写入服务，刷新后丢弃。
              </p>
            )}
          </Panel>
          <Panel
            title="对白与声音"
            description={`${shot.character} · 声线授权待核对`}
          >
            <p className="text-base leading-8">“{shot.dialogue}”</p>
            <Button variant="ghost" className="mt-4" asChild>
              <Link href={`/projects/${projectId}/episodes/${episodeId}/audio`}>
                进入配音工作台 →
              </Link>
            </Button>
          </Panel>
        </div>
      </div>
      <Panel title="版本与操作历史">
        <DataTable
          caption="镜头历史"
          columns={["时间", "操作", "版本"]}
          rows={[
            ["今天 10:42", "关键帧候选登记（样例）", "镜头 v3 / 候选 A、B"],
            ["今天 10:20", "调整画面描述（样例）", "镜头 v2 → v3"],
          ]}
        />
      </Panel>
      <Dialog open={impact} onOpenChange={setImpact}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>候选选定影响</DialogTitle>
            <DialogDescription>
              候选 A → 候选 {candidate}。这只是前端提案，不改变正式镜头。
            </DialogDescription>
          </DialogHeader>
          <ul className="list-inside list-disc space-y-2 text-sm leading-7">
            <li>镜头 {shot.number} 的关键帧引用拟更新。</li>
            <li>下游视频和故事板可能需要重新核对。</li>
            <li>已有媒体保留；重新生成需要新的报价确认。</li>
          </ul>
          <Button
            disabled={readOnly}
            onClick={() => {
              setProposed(candidate);
              setImpact(false);
            }}
          >
            仅在本页预览选定
          </Button>
        </DialogContent>
      </Dialog>
    </div>
  );
}
export function AudioPage() {
  const { readOnly } = usePreviewQuery();
  const [selected, setSelected] = useState<string>(shots[0].id);
  const [emotion, setEmotion] = useState("自然");
  const [speed, setSpeed] = useState("1.0");
  const shot = shots.find((s) => s.id === selected)!;
  return (
    <div className="space-y-8">
      <PageHeading
        title="让角色拥有声音。"
        description="台词、发声人、声线授权与情绪逐条核对。此处没有真实音频合成。"
        action={
          <QuotePreview
            target="第一集 · 配音"
            label="预览配音报价"
            kind="audio"
            disabled={readOnly}
          />
        }
      />
      <div className="grid gap-7 xl:grid-cols-[1.3fr_1fr]">
        <Panel title="台词列表">
          <ul className="space-y-3">
            {shots.map((s) => (
              <li key={s.id}>
                <Button
                  variant={s.id === selected ? "secondary" : "ghost"}
                  className="h-auto w-full justify-start py-4 text-left"
                  onClick={() => setSelected(s.id)}
                  aria-pressed={s.id === selected}
                >
                  <span>
                    <span className="text-xs text-muted-foreground">
                      镜头 {s.number} · {s.character}
                    </span>
                    <span className="mt-2 block text-sm whitespace-normal">
                      {s.dialogue}
                    </span>
                  </span>
                </Button>
              </li>
            ))}
          </ul>
        </Panel>
        <Panel title="声音配置" description="样例声线 · 授权待核对">
          <FieldGroup>
            <p className="text-sm">
              {shot.character}：{shot.dialogue}
            </p>
            <ChoiceField
              id="voice-emotion"
              label="情绪"
              value={emotion}
              options={["自然", "好奇", "平静"]}
              onChange={setEmotion}
              disabled={readOnly}
            />
            <ChoiceField
              id="voice-speed"
              label="语速"
              value={speed}
              options={["0.8", "1.0", "1.2"]}
              onChange={setSpeed}
              disabled={readOnly}
            />
            <DraftNotice changed={emotion !== "自然" || speed !== "1.0"} />
            <Button disabled>试听服务待接入</Button>
          </FieldGroup>
        </Panel>
      </div>
    </div>
  );
}
