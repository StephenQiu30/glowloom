"use client";

import Image from "next/image";
import Link from "next/link";
import { useState } from "react";
import {
  Upload,
  Play,
  ImageIcon,
  Search,
  ArrowUpRight,
  AudioLines,
  UserRound,
  Mountain,
  Package,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { entities, media } from "./data";
import {
  PageHeading,
  Panel,
  DataTable,
  EmptyMessage,
  usePreviewQuery,
} from "./workbench-components";

const categories = {
  characters: "角色",
  scenes: "场景",
  props: "道具",
  voices: "声音",
};
const categoryIcons = {
  characters: UserRound,
  scenes: Mountain,
  props: Package,
  voices: AudioLines,
};
export function BiblePage({ projectId }: { projectId: string }) {
  const { params, set } = usePreviewQuery();
  const requestedCategory = params.get("category") ?? "";
  const category = Object.keys(entities).includes(requestedCategory)
    ? requestedCategory
    : "characters";
  return (
    <div className="space-y-6">
      <PageHeading
        title="故事资产"
        description="管理角色、场景、道具与声音，核对版本后用于单集与镜头。"
      />
      <Tabs value={category} onValueChange={(v) => set("category", v)}>
        <TabsList variant="line" className="w-full justify-start gap-4">
          {Object.entries(categories).map(([key, label]) => (
            <TabsTrigger key={key} value={key}>
              {label}
            </TabsTrigger>
          ))}
        </TabsList>
        {Object.entries(entities).map(([key, list]) => (
          <TabsContent key={key} value={key} className="mt-5">
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
              {list.map((e) => {
                const Icon = categoryIcons[key as keyof typeof categoryIcons];
                return (
                  <Card
                    key={e.id}
                    className="gap-0 border-0 bg-card py-0 shadow-none ring-0"
                  >
                    <Dialog>
                      <DialogTrigger asChild>
                        <Button
                          variant="ghost"
                          className="h-auto w-full rounded-b-none p-0"
                          aria-label={`查看 ${e.name} 资产详情`}
                        >
                          <span className="flex aspect-[16/10] w-full flex-col items-center justify-center gap-3 rounded-t-xl bg-muted/50">
                            <Icon
                              aria-hidden="true"
                              className="size-9 text-primary/70"
                            />
                            <span className="text-xs font-normal text-muted-foreground">
                              {categories[key as keyof typeof categories]} ·
                              样例资产
                            </span>
                          </span>
                        </Button>
                      </DialogTrigger>
                      <DialogContent className="sm:max-w-lg">
                        <DialogHeader>
                          <DialogTitle>{e.name}</DialogTitle>
                          <DialogDescription>
                            {e.detail} · 样例资产
                          </DialogDescription>
                        </DialogHeader>
                        <DataTable
                          caption="资产详情"
                          columns={["字段", "内容"]}
                          rows={[
                            ["版本", e.version],
                            ["选定状态", e.status],
                            [
                              "授权",
                              key === "voices"
                                ? "需要声线授权证据，当前未提供"
                                : "样例素材，仅用于界面",
                            ],
                            ["引用范围", "第一集 / 镜头 01"],
                          ]}
                        />
                        <Button variant="secondary" asChild>
                          <Link
                            href={`/projects/${projectId}/episodes/ep-01/assets`}
                          >
                            查看单集资产
                          </Link>
                        </Button>
                        <Button disabled>锁定与版本服务待接入</Button>
                      </DialogContent>
                    </Dialog>
                    <CardContent className="space-y-3 p-4">
                      <div className="flex items-center justify-between gap-3">
                        <h2 className="truncate text-sm font-medium">
                          {e.name}
                        </h2>
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {e.version}
                        </span>
                      </div>
                      <p className="text-xs leading-5 text-muted-foreground">
                        {e.detail}
                      </p>
                      <Badge variant="secondary">{e.status}</Badge>
                    </CardContent>
                  </Card>
                );
              })}
            </div>
          </TabsContent>
        ))}
      </Tabs>
    </div>
  );
}
export function AssetsPage({
  projectId,
  episodeId,
}: {
  projectId: string;
  episodeId: string;
}) {
  return (
    <div className="space-y-6">
      <PageHeading
        title="单集资产"
        description="核对引用与锁定版本。素材缺失时先补齐，再进入分镜与生成。"
        action={
          <Button asChild>
            <Link href={`/projects/${projectId}/episodes/${episodeId}/shots`}>
              进入分镜
              <ArrowUpRight data-icon="inline-end" />
            </Link>
          </Button>
        }
      />
      <Panel title="资产清单">
        <DataTable
          caption="单集资产引用"
          columns={["资产", "类型", "版本", "状态", "关联"]}
          rows={[
            [
              "林夏",
              "角色 / 主服装",
              "v3",
              <Badge key="lin" variant="secondary">
                已锁定
              </Badge>,
              "镜头 01 / 02 / 03",
            ],
            ["旧港码头", "场景", "v2", "已锁定", "镜头 01"],
            ["泛黄信封", "道具", "v2", "已锁定", "镜头 02"],
            [
              "邮局内景",
              "场景",
              "v1",
              <Badge key="post" variant="destructive">
                待选定
              </Badge>,
              "镜头 02",
            ],
            ["林夏声线", "声音", "v1", "授权待核对", "配音 01 / 03"],
          ]}
        />
      </Panel>
      <Panel title="待处理事项" description="场景待选定，声音授权待核对。">
        <div className="flex flex-wrap gap-3">
          <Button variant="secondary" asChild>
            <Link href={`/projects/${projectId}/bible?category=scenes`}>
              核对场景
            </Link>
          </Button>
          <Button variant="secondary" asChild>
            <Link href={`/projects/${projectId}/bible?category=voices`}>
              核对声音授权
            </Link>
          </Button>
        </div>
      </Panel>
    </div>
  );
}
export function MediaPage() {
  const { params, set, readOnly } = usePreviewQuery();
  const q = params.get("q") ?? "";
  const type = ["image", "video", "audio"].includes(params.get("type") ?? "")
    ? params.get("type")!
    : "all";
  const search = q.trim().toLocaleLowerCase();
  const list = media.filter(
    (m) =>
      (type === "all" || type === m.type) &&
      `${m.name} ${m.origin} ${m.id}`.toLocaleLowerCase().includes(search),
  );
  const [fileName, setFileName] = useState("");
  return (
    <div className="space-y-6">
      <PageHeading
        title="素材库"
        description="查看图像、视频与音频，追溯来源、审核状态和引用。当前为内置样例。"
        action={
          <Dialog>
            <DialogTrigger asChild>
              <Button disabled={readOnly}>
                <Upload data-icon="inline-start" />
                上传预览
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>上传素材预览</DialogTitle>
                <DialogDescription>
                  仅显示所选文件名，不读取文件内容，不发起上传。
                </DialogDescription>
              </DialogHeader>
              <Field>
                <FieldLabel htmlFor="upload-media">选择本地素材</FieldLabel>
                <Input
                  id="upload-media"
                  type="file"
                  accept="image/*,video/*,audio/*"
                  disabled={readOnly}
                  onChange={(e) => setFileName(e.target.files?.[0]?.name ?? "")}
                />
              </Field>
              {fileName && (
                <p role="status">已选择：{fileName}。上传服务待接入。</p>
              )}
              <Button disabled>上传服务待接入</Button>
            </DialogContent>
          </Dialog>
        }
      />
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <ToggleGroup
          type="single"
          aria-label="素材类型"
          value={type}
          className="max-w-full flex-wrap gap-1"
          onValueChange={(v) => {
            if (v) set("type", v);
          }}
        >
          <ToggleGroupItem
            value="all"
            className="data-[state=on]:bg-primary/10 data-[state=on]:text-primary"
          >
            全部
          </ToggleGroupItem>
          <ToggleGroupItem
            value="image"
            className="data-[state=on]:bg-primary/10 data-[state=on]:text-primary"
          >
            图像
          </ToggleGroupItem>
          <ToggleGroupItem
            value="video"
            className="data-[state=on]:bg-primary/10 data-[state=on]:text-primary"
          >
            视频
          </ToggleGroupItem>
          <ToggleGroupItem
            value="audio"
            className="data-[state=on]:bg-primary/10 data-[state=on]:text-primary"
          >
            音频
          </ToggleGroupItem>
        </ToggleGroup>
        <div className="relative w-full sm:max-w-xs">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground"
          />
          <Input
            aria-label="搜索素材"
            className="h-9 rounded-full border-0 bg-muted/50 pl-9"
            placeholder="搜索素材名称或来源"
            value={q}
            onChange={(e) => set("q", e.target.value)}
          />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">{list.length} 项素材</p>
      {list.length === 0 ? (
        <EmptyMessage
          title="没有匹配的素材"
          description="尝试其他类型或搜索词。"
        />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
          {list.map((m) => (
            <Card
              key={m.id}
              className="gap-0 border-0 bg-card py-0 shadow-none ring-0"
            >
              <Dialog>
                <DialogTrigger asChild>
                  <Button
                    variant="ghost"
                    className="relative h-auto w-full overflow-hidden rounded-t-xl rounded-b-none p-0"
                    aria-label={`预览与来源：${m.name}`}
                  >
                    <span className="relative block aspect-video w-full overflow-hidden bg-muted">
                      <Image
                        src="/examples/media/scene.svg"
                        alt="内置素材样例：山景插画，非真实生成产物"
                        fill
                        sizes="(max-width: 640px) 90vw, (max-width: 1280px) 45vw, 25vw"
                        className="object-cover"
                      />
                      {m.type === "video" ? (
                        <Play
                          aria-hidden="true"
                          className="absolute top-3 right-3 size-5 text-white"
                        />
                      ) : (
                        <ImageIcon
                          aria-hidden="true"
                          className="absolute top-3 right-3 size-5 text-white"
                        />
                      )}
                      <span className="absolute bottom-2 left-2 rounded-md bg-black/60 px-2 py-1 text-[10px] font-normal text-white">
                        {m.type === "video" ? "视频" : "图像"} · 样例
                      </span>
                    </span>
                  </Button>
                </DialogTrigger>
                <DialogContent className="sm:max-w-2xl">
                  <DialogHeader>
                    <DialogTitle>{m.name}</DialogTitle>
                    <DialogDescription>
                      {m.origin} · {m.id} · 所有媒体是内置演示素材。
                    </DialogDescription>
                  </DialogHeader>
                  {m.type === "video" ? (
                    <video
                      src="/examples/media/preview.mp4"
                      poster="/examples/media/scene.svg"
                      controls
                      preload="none"
                      className="aspect-video w-full rounded-lg"
                      aria-label="内置视频样例"
                    />
                  ) : (
                    <Image
                      width={560}
                      height={340}
                      src="/examples/media/scene.svg"
                      alt="内置山景插画样例"
                      className="w-full rounded-lg"
                    />
                  )}
                  <DataTable
                    caption="素材来源与引用"
                    columns={["来源", "审核", "引用"]}
                    rows={[
                      [m.origin, m.status, `${m.refs} 个业务引用（样例）`],
                    ]}
                  />
                </DialogContent>
              </Dialog>
              <CardContent className="space-y-3 p-4">
                <h2 className="truncate text-sm font-medium">{m.name}</h2>
                <div className="flex items-center justify-between gap-2">
                  <Badge variant="secondary">{m.status}</Badge>
                  <span className="text-xs text-muted-foreground">
                    {m.refs} 个引用
                  </span>
                </div>
                <p className="text-xs text-muted-foreground">{m.origin}</p>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
