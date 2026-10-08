"use client";

import { useState } from "react";
import Link from "next/link";
import {
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Check,
  Grid2X2,
  Pause,
  Play,
  Plus,
} from "lucide-react";
import { cn } from "cn";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableCell,
  TableHead,
} from "@/components/ui/table";
import { Progress } from "@/components/ui/progress";
import {
  ProductShell,
  PageHeading,
  Choice,
  Placeholder,
  IconButton,
  MoreMenu,
  LocalDialog,
  demoNotice,
} from "./shared";
import { screenHref } from "./screens";
export const shotFixtures = [
  {
    id: "02-01",
    duration: "3.9s",
    mode: "图生视频",
    status: "已选定",
    video: "已选定",
    text: "远景，雨夜码头云雾缭绕。",
  },
  {
    id: "02-02",
    duration: "4.2s",
    mode: "图生视频",
    status: "已选定",
    video: "候选 2",
    text: "林舟停步走入画面，回头望向灯塔。",
  },
  {
    id: "02-03",
    duration: "2.8s",
    mode: "首尾帧",
    status: "已选定",
    video: "生成中",
    text: "特写，信封被雨打湿。",
  },
  {
    id: "02-04",
    duration: "4.0s",
    mode: "图生视频",
    status: "候选 2",
    video: "未生成",
    text: "中景推进，林舟在雨中抬头回望。冷色深水，背景灯塔虚焦。",
  },
  {
    id: "02-05",
    duration: "3.8s",
    mode: "全能参考",
    status: "生成中",
    video: "未生成",
    text: "反打，灯塔摇曳的光扫过雾面。",
  },
  {
    id: "02-06",
    duration: "5.0s",
    mode: "图生视频",
    status: "失败",
    video: "未生成",
    text: "雨中的旧码头，镜头缓慢拉远。",
  },
  {
    id: "02-07",
    duration: "3.2s",
    mode: "图生视频",
    status: "未生成",
    video: "未生成",
    text: "林舟低头查看信封。",
  },
  {
    id: "02-08",
    duration: "2.5s",
    mode: "首尾帧",
    status: "未生成",
    video: "未生成",
    text: "水面泛起微弱的涟漪。",
  },
  {
    id: "02-09",
    duration: "4.9s",
    mode: "全能参考",
    status: "生成中",
    video: "未生成",
    text: "汽笛响起，渔船驶离码头。",
  },
  {
    id: "02-10",
    duration: "3.8s",
    mode: "图生视频",
    status: "未生成",
    video: "未生成",
    text: "远处的灯塔在雾中渐渐消失。",
  },
];
function ShotBadge({ label, status }: { label: string; status: string }) {
  return (
    <Badge
      variant={
        status === "已选定"
          ? "default"
          : status === "生成中"
            ? "warning"
            : status === "失败"
              ? "destructive"
              : "muted"
      }
    >
      {label} · {status}
    </Badge>
  );
}
export function StoryboardPage() {
  const [selected, setSelected] = useState(["02-04", "02-05", "02-06"]);
  const [view, setView] = useState("故事板");
  const [status, setStatus] = useState("状态：全部");
  const [mode, setMode] = useState("模式：全部");
  const [batch, setBatch] = useState(false);
  const [generated, setGenerated] = useState<string[]>([]);
  const [duration, setDuration] = useState("4.0s");
  const shots = shotFixtures.filter(
    (shot) =>
      (status.endsWith("全部") || status.includes(shot.status)) &&
      (mode.endsWith("全部") || mode.includes(shot.mode)),
  );
  return (
    <ProductShell screen="storyboard" contextual>
      <PageHeading
        title="第 3 集 · 分镜"
        description="场 2 · 旧码头 · 12 个镜头"
      >
        <ToggleGroup
          type="single"
          value={view}
          onValueChange={(value) => value && setView(value)}
          aria-label="分镜视图"
        >
          <ToggleGroupItem value="镜头表">镜头表</ToggleGroupItem>
          <ToggleGroupItem value="故事板">故事板</ToggleGroupItem>
        </ToggleGroup>
        <Button variant="outline" asChild>
          <Link href={screenHref("canvas")}>
            <Grid2X2 data-icon="inline-start" />
            在画布中打开
          </Link>
        </Button>
      </PageHeading>
      <div className="mb-6 flex flex-wrap items-center gap-3 rounded-lg bg-surface-1 p-3">
        <span className="text-xs">已选 {selected.length} 项</span>
        <Button variant="ghost" size="xs" onClick={() => setSelected([])}>
          取消选择
        </Button>
        <Choice
          value={status}
          onChange={setStatus}
          label="镜头状态"
          options={[
            "状态：全部",
            "状态：已选定",
            "状态：未生成",
            "状态：生成中",
            "状态：失败",
          ]}
        />
        <Choice
          value={mode}
          onChange={setMode}
          label="生成模式"
          options={[
            "模式：全部",
            "模式：图生视频",
            "模式：首尾帧",
            "模式：全能参考",
          ]}
        />
        <div className="ml-auto flex gap-2">
          <Button
            variant="outline"
            disabled={!selected.length}
            onClick={() => setBatch(true)}
          >
            批量改参数
          </Button>
          <Button
            disabled={!selected.length}
            onClick={() => {
              setGenerated([...new Set([...generated, ...selected])]);
              demoNotice(`已为 ${selected.length} 个镜头创建演示任务`);
            }}
          >
            批量生成 {selected.length} 项
          </Button>
        </div>
      </div>
      {view === "故事板" ? (
        <div className="grid grid-cols-2 gap-x-5 gap-y-7 sm:grid-cols-3 xl:grid-cols-6">
          {shots.map((shot) => (
            <Card
              variant="project"
              key={shot.id}
              data-selected={selected.includes(shot.id) || undefined}
            >
              <div className="relative">
                <Link
                  href={screenHref("shot")}
                  aria-label={`打开镜头 ${shot.id}`}
                >
                  <Placeholder className="aspect-[9/16]">
                    <span className="absolute right-2 bottom-2 rounded-sm bg-background px-1 font-mono text-[10px]">
                      {shot.duration}
                    </span>
                  </Placeholder>
                </Link>
                <Checkbox
                  aria-label={`选择镜头 ${shot.id}`}
                  checked={selected.includes(shot.id)}
                  onCheckedChange={(checked) =>
                    setSelected((current) =>
                      checked
                        ? [...current, shot.id]
                        : current.filter((id) => id !== shot.id),
                    )
                  }
                  className="absolute top-2 left-2"
                />
              </div>
              <CardHeader className="mt-2">
                <CardTitle>
                  <div className="flex items-center justify-between gap-1">
                    <Link
                      href={screenHref("shot")}
                      className="font-mono text-xs"
                    >
                      {shot.id}
                    </Link>
                    <span className="text-[10px] font-normal text-muted-foreground">
                      {shot.mode}
                    </span>
                  </div>
                </CardTitle>
              </CardHeader>
              <CardContent className="mt-2 flex flex-wrap gap-1">
                <ShotBadge
                  label="画面"
                  status={generated.includes(shot.id) ? "生成中" : shot.status}
                />
                <ShotBadge label="视频" status={shot.video} />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        <div className="rounded-lg bg-surface-1">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>选择</TableHead>
                <TableHead>编号</TableHead>
                <TableHead>画面描述</TableHead>
                <TableHead>时长</TableHead>
                <TableHead>状态</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {shots.map((shot) => (
                <TableRow key={shot.id}>
                  <TableCell>
                    <Checkbox
                      aria-label={`选择镜头 ${shot.id}`}
                      checked={selected.includes(shot.id)}
                      onCheckedChange={(checked) =>
                        setSelected((current) =>
                          checked
                            ? [...current, shot.id]
                            : current.filter((id) => id !== shot.id),
                        )
                      }
                    />
                  </TableCell>
                  <TableCell className="font-mono">{shot.id}</TableCell>
                  <TableCell>{shot.text}</TableCell>
                  <TableCell>{shot.duration}</TableCell>
                  <TableCell>
                    <ShotBadge label="画面" status={shot.status} />
                  </TableCell>
                  <TableCell>
                    <Button asChild variant="ghost">
                      <Link href={screenHref("shot")}>打开</Link>
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      <LocalDialog
        open={batch}
        onOpenChange={setBatch}
        title={`修改 ${selected.length} 个镜头参数`}
        onConfirm={() => demoNotice("镜头参数已更新")}
      >
        <FieldGroup>
          <Field>
            <FieldLabel>时长</FieldLabel>
            <Choice
              value={duration}
              onChange={setDuration}
              options={["3.0s", "4.0s", "5.0s", "10.0s"]}
              label="批量镜头时长"
            />
          </Field>
        </FieldGroup>
      </LocalDialog>
    </ProductShell>
  );
}
function Waveform({ playing }: { playing: boolean }) {
  return (
    <div
      className="flex h-7 items-center gap-[3px]"
      aria-label={playing ? "演示音频播放中" : "音频波形"}
    >
      {[
        4, 10, 14, 22, 17, 26, 12, 20, 25, 11, 18, 24, 15, 9, 20, 13, 6, 12,
      ].map((height, index) => (
        <span
          key={index}
          className={cn(
            "w-[2px] rounded-full bg-muted-foreground",
            playing && "animate-pulse",
          )}
          style={{ height }}
        />
      ))}
    </div>
  );
}
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
                      <Placeholder className="aspect-[9/16]" kind="video">
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
                            <Play data-icon="inline-start" />
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
                  <Waveform playing={voice} />
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
