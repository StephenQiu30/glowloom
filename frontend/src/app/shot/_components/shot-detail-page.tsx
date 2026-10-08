"use client";

import { shotFixtures } from "@/components/storyboard/mock-shots";
import { ProductShell } from "@/components/layout/product-shell";
import { IconButton } from "@/components/controls/icon-button";
import { Choice } from "@/components/forms/choice";
import { demoNotice } from "@/components/feedback/demo-notice";
import { Placeholder } from "@/components/media/placeholder";
import { MoreMenu } from "@/components/controls/more-menu";
import { AudioWaveform } from "@/components/media/audio-waveform";
import { LocalDialog } from "@/components/controls/local-dialog";
import { useState } from "react";
import Link from "next/link";
import {
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Check,
  Pause,
  Play,
  Plus,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Progress } from "@/components/ui/progress";
import { screenHref } from "@/components/layout/routes";

export function ShotDetailPage() {
  const [index, setIndex] = useState(3);
  const [description, setDescription] = useState(shotFixtures[3].text);
  const [mode, setMode] = useState("图生视频");
  const [scene, setScene] = useState("中景");
  const [motion, setMotion] = useState("推镜");
  const [duration, setDuration] = useState("4.0s");
  const [model, setModel] = useState("[视频模型]");
  const [selected, setSelected] = useState<string | null>(null);
  const [playing, setPlaying] = useState<string[]>([]);
  const [voice, setVoice] = useState(false);
  const [version, setVersion] = useState("v3");
  const [reference, setReference] = useState(false);
  const shot = shotFixtures[index];
  function navigate(delta: number) {
    const next = Math.min(shotFixtures.length - 1, Math.max(0, index + delta));
    setIndex(next);
    setDescription(shotFixtures[next].text);
    setSelected(null);
  }
  return (
    <ProductShell screen="shot" fullBleed>
      <div className="flex min-h-svh flex-col p-4">
        <header className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <IconButton
              icon={ArrowLeft}
              label="返回分镜"
              href={screenHref("storyboard")}
            />
            <h1 className="font-mono text-xl font-semibold">S03-{shot.id}</h1>
            <Badge variant="secondary">候选 2</Badge>
          </div>
          <div className="flex gap-2">
            <IconButton
              label="上一个镜头"
              icon={ChevronLeft}
              onClick={() => navigate(-1)}
              disabled={index === 0}
            />
            <IconButton
              label="下一个镜头"
              icon={ChevronRight}
              onClick={() => navigate(1)}
              disabled={index === shotFixtures.length - 1}
            />
            <Button variant="ghost" asChild>
              <Link href={screenHref("canvas")}>在画布中定位</Link>
            </Button>
          </div>
        </header>
        <div className="grid flex-1 items-stretch gap-4 lg:grid-cols-[300px_minmax(0,1fr)_280px]">
          <Card variant="panel">
            <CardHeader>
              <CardTitle>镜头参数</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-1 flex-col gap-5">
              <ToggleGroup
                type="single"
                value={mode}
                onValueChange={(value) => value && setMode(value)}
                aria-label="生成方式"
                size="sm"
              >
                {["图生视频", "首尾帧", "全能参考"].map((value) => (
                  <ToggleGroupItem key={value} value={value}>
                    {value}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
              <FieldGroup className="gap-4">
                <Field>
                  <FieldLabel htmlFor="shot-description">画面描述</FieldLabel>
                  <Textarea
                    id="shot-description"
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    rows={5}
                    className="min-h-28"
                  />
                </Field>
                <div className="grid grid-cols-2 gap-3">
                  <Field>
                    <FieldLabel>景别</FieldLabel>
                    <Choice
                      label="景别"
                      value={scene}
                      onChange={setScene}
                      options={["远景", "中景", "近景", "特写"]}
                      className="w-full"
                    />
                  </Field>
                  <Field>
                    <FieldLabel>运镜</FieldLabel>
                    <Choice
                      label="运镜"
                      value={motion}
                      onChange={setMotion}
                      options={["推镜", "拉镜", "固定", "跟随"]}
                      className="w-full"
                    />
                  </Field>
                  <Field>
                    <FieldLabel>时长</FieldLabel>
                    <Choice
                      label="时长"
                      value={duration}
                      onChange={setDuration}
                      options={["3.0s", "4.0s", "5.0s"]}
                      className="w-full"
                    />
                  </Field>
                  <Field>
                    <FieldLabel>模型</FieldLabel>
                    <Choice
                      label="视频模型"
                      value={model}
                      onChange={setModel}
                      options={["[视频模型]", "[视频模型 A]", "[视频模型 B]"]}
                      className="w-full"
                    />
                  </Field>
                </div>
              </FieldGroup>
              <div className="flex items-center justify-between">
                <h2 className="text-sm font-medium">参考组合</h2>
                <span className="text-[10px] text-muted-foreground">
                  图片 2/9 · 视频 0/3 · 音频 1/3
                </span>
              </div>
              {["林舟 · 雨夜造型", "旧码头 · 夜"].map((label, i) => (
                <div
                  key={label}
                  className="flex items-center gap-3 rounded-md bg-surface-2 p-3"
                >
                  <span className="size-8 rounded-sm bg-surface-3" />
                  <div className="flex-1 text-xs">
                    {label}
                    <p className="mt-1 text-[10px] text-muted-foreground">
                      v2 · 已锁定
                    </p>
                  </div>
                  <Badge variant="secondary">{i ? "场景" : "角色"}</Badge>
                </div>
              ))}
              <Button
                variant="outline"
                size="sm"
                onClick={() => setReference(true)}
              >
                <Plus data-icon="inline-start" />
                添加参考：设定集 / 本地素材 / 台词音频
              </Button>
              <Button
                className="mt-auto w-full"
                onClick={() => {
                  setSelected(null);
                  demoNotice("已生成 2 个演示候选");
                }}
              >
                生成 2 个候选
              </Button>
            </CardContent>
          </Card>
          <Card variant="panel">
            <CardHeader>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex gap-3">
                  <CardTitle>候选</CardTitle>
                  <Badge variant="muted">失败的 · 已隐藏</Badge>
                  <Badge variant="secondary">视频 · 2</Badge>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setPlaying(playing.length ? [] : ["A", "B"])}
                >
                  {playing.length ? (
                    <Pause data-icon="inline-start" />
                  ) : (
                    <Play data-icon="inline-start" />
                  )}
                  同步播放
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-8">
                {["A", "B"].map((candidate) => (
                  <div key={candidate}>
                    <div className="relative">
                      <Placeholder
                        className="aspect-[9/16]"
                        kind="video"
                        showIcon={false}
                      >
                        <Badge
                          variant="secondary"
                          className="absolute top-2 left-2"
                        >
                          {candidate}
                        </Badge>
                        <Button
                          variant="ghost"
                          size="icon-lg"
                          className="absolute"
                          aria-label={`${playing.includes(candidate) ? "暂停" : "播放"}候选 ${candidate}`}
                          onClick={() =>
                            setPlaying((current) =>
                              current.includes(candidate)
                                ? current.filter((value) => value !== candidate)
                                : [...current, candidate],
                            )
                          }
                        >
                          {playing.includes(candidate) ? (
                            <Pause data-icon="inline-start" />
                          ) : (
                            <Play
                              data-icon="inline-start"
                              fill="currentColor"
                            />
                          )}
                        </Button>
                        <div className="absolute inset-x-3 bottom-3">
                          <Progress
                            value={playing.includes(candidate) ? 65 : 37}
                            aria-label={`候选 ${candidate} 播放进度`}
                          />
                        </div>
                      </Placeholder>
                    </div>
                    <div className="mt-3 flex gap-2">
                      <Button
                        className="flex-1"
                        variant={
                          selected === candidate ? "secondary" : "default"
                        }
                        onClick={() => {
                          setSelected(candidate);
                          demoNotice(`已选定候选 ${candidate}`);
                        }}
                      >
                        {selected === candidate ? (
                          <Check data-icon="inline-start" />
                        ) : null}
                        {selected === candidate ? "已选定" : "选定"}
                      </Button>
                      <MoreMenu
                        label={`候选 ${candidate} 更多操作`}
                        items={[
                          {
                            label: "保存到素材库",
                            action: () => demoNotice("候选已保存到素材库"),
                          },
                          {
                            label: "复制参数",
                            action: () => demoNotice("候选参数已复制"),
                          },
                        ]}
                      />
                    </div>
                  </div>
                ))}
              </div>
              <p className="mt-4 text-xs text-muted-foreground">
                选定后将替换当前结果；依赖当前结果的下游将标为失效。
              </p>
            </CardContent>
          </Card>
          <Card variant="panel">
            <CardHeader>
              <CardTitle>台词与音频</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <div className="rounded-lg bg-surface-2 p-3">
                <div className="flex justify-between text-xs">
                  <span>林舟</span>
                  <Badge>已选定</Badge>
                </div>
                <p className="my-4 text-sm">“没封好，本来就寄不到吧。”</p>
                <div className="flex items-center gap-2">
                  <IconButton
                    label={voice ? "暂停台词" : "播放台词"}
                    icon={voice ? Pause : Play}
                    onClick={() => setVoice(!voice)}
                    size="icon-sm"
                  />
                  <AudioWaveform playing={voice} />
                  <span className="ml-auto font-mono text-[10px]">2.8s</span>
                </div>
                <div className="mt-3 flex flex-wrap gap-1">
                  <Badge variant="muted">音色：林舟</Badge>
                  <Badge variant="muted">语速 1.0</Badge>
                  <Badge variant="muted">低沉</Badge>
                </div>
              </div>
              <div className="rounded-lg bg-surface-2 p-3">
                <div className="flex justify-between text-sm">
                  <span>环境音</span>
                  <Badge variant="muted">未生成</Badge>
                </div>
                <p className="mt-2 text-xs leading-5 text-muted-foreground">
                  雨声、远处汽笛，可从本地导入音频。
                </p>
              </div>
            </CardContent>
          </Card>
        </div>
        <Card variant="panel" className="mt-4">
          <CardHeader>
            <CardTitle>版本记录</CardTitle>
          </CardHeader>
          <CardContent>
            <ToggleGroup
              type="single"
              value={version}
              onValueChange={(value) => {
                if (value) {
                  setVersion(value);
                  demoNotice(`正在查看 ${value}`);
                }
              }}
              aria-label="镜头版本"
              className="w-full flex-wrap"
            >
              {["v3", "v2", "v1"].map((value, i) => (
                <ToggleGroupItem
                  key={value}
                  value={value}
                  className="h-auto min-w-40 flex-1 justify-start p-3"
                >
                  <span className="text-left">
                    <span className="block">
                      {value}
                      {!i ? " · 当前" : ""}
                    </span>
                    <span className="mt-1 block text-[10px] text-muted-foreground">
                      {i === 0
                        ? "调整镜头描述，时长 3s → 4s"
                        : i === 1
                          ? "新增参考：旧码头 · 夜"
                          : "初始剧本拆分生成"}
                    </span>
                  </span>
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </CardContent>
        </Card>
      </div>
      <LocalDialog
        open={reference}
        onOpenChange={setReference}
        title="添加参考"
        onConfirm={() => demoNotice("参考素材已添加")}
      >
        <p className="text-sm text-muted-foreground">
          林舟 · 雨夜造型、旧码头 · 夜、林舟台词音频已加入参考组合。
        </p>
      </LocalDialog>
    </ProductShell>
  );
}
