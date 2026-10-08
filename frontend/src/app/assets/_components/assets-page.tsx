"use client";

import { ProductShell } from "@/components/layout/product-shell";
import { IconButton } from "@/components/controls/icon-button";
import { demoNotice } from "@/components/feedback/demo-notice";
import { PageHeading } from "@/components/layout/page-heading";
import { SearchInput } from "@/components/forms/search-input";
import { Choice } from "@/components/forms/choice";
import { Placeholder } from "@/components/media/placeholder";
import { EmptySearch } from "@/components/feedback/empty-search";
import { useRef, useState } from "react";
import { cn } from "cn";
import { Clock3, Folder, Grid2X2, Plus, Star, Upload, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Progress } from "@/components/ui/progress";

const initialAssets = [
  {
    name: "码头雨夜 · 远景",
    kind: "图片",
    ratio: "16:9",
    used: 2,
    folder: "场景参考",
  },
  {
    name: "灯塔 · 黄昏",
    kind: "图片",
    ratio: "9:16",
    used: 0,
    folder: "场景参考",
  },
  { name: "雨声渐强", kind: "音频", ratio: "0:42", used: 1, folder: "环境音" },
  { name: "旧信封特写", kind: "图片", ratio: "1:1", used: 0, folder: "服装" },
  {
    name: "渔港俯拍",
    kind: "视频",
    ratio: "0:26",
    used: 0,
    folder: "场景参考",
  },
  { name: "风衣面料", kind: "图片", ratio: "4:3", used: 1, folder: "服装" },
  {
    name: "旁白初稿",
    kind: "文本",
    ratio: "1.2k 字",
    used: 0,
    folder: "未分类",
  },
  {
    name: "城市夜景",
    kind: "图片",
    ratio: "16:9",
    used: 0,
    folder: "场景参考",
  },
  { name: "汽笛", kind: "音频", ratio: "0:26", used: 0, folder: "环境音" },
  { name: "码头木纹", kind: "图片", ratio: "3:4", used: 1, folder: "场景参考" },
];

export function AssetsPage() {
  const [assets, setAssets] = useState(initialAssets);
  const [search, setSearch] = useState("");
  const [kind, setKind] = useState("全部");
  const [scope, setScope] = useState("个人");
  const [folder, setFolder] = useState("全部");
  const [sort, setSort] = useState("最近更新");
  const [selected, setSelected] = useState<string | null>(
    initialAssets[0].name,
  );
  const [favorites, setFavorites] = useState([initialAssets[0].name]);
  const [uploading, setUploading] = useState(true);
  const [page, setPage] = useState(1);
  const uploadRef = useRef<HTMLInputElement>(null);
  const visible = assets.filter(
    (asset) =>
      asset.name.includes(search) &&
      (kind === "全部" || kind === asset.kind) &&
      (folder === "全部" ||
        folder === "最近 30 天" ||
        (folder === "收藏" && favorites.includes(asset.name)) ||
        asset.folder === folder),
  );
  if (sort === "名称")
    visible.sort((a, b) => a.name.localeCompare(b.name, "zh-CN"));
  const asset = assets.find((item) => item.name === selected);
  return (
    <ProductShell screen="assets" fullBleed>
      <div className="flex min-h-svh flex-col lg:flex-row">
        <aside className="flex w-full shrink-0 flex-col gap-6 bg-surface-1 p-5 lg:sticky lg:top-0 lg:h-svh lg:w-[260px]">
          <ToggleGroup
            type="single"
            value={scope}
            onValueChange={(value) => value && setScope(value)}
            aria-label="素材范围"
            className="w-full rounded-md bg-surface-2 p-0.5"
          >
            <ToggleGroupItem value="个人" className="flex-1">
              个人
            </ToggleGroupItem>
            <ToggleGroupItem value="项目" className="flex-1">
              项目
            </ToggleGroupItem>
          </ToggleGroup>
          <nav
            className="grid grid-cols-2 gap-1 lg:flex lg:flex-col"
            aria-label="素材导航"
          >
            {[
              ["全部", "128", Grid2X2],
              ["收藏", "14", Star],
              ["最近 30 天", "37", Clock3],
              ["未分类", "22", Folder],
            ].map(([label, count, Icon]) => {
              const NavIcon = Icon as typeof Folder;
              return (
                <Button
                  key={label as string}
                  variant="navigation"
                  data-active={folder === label || undefined}
                  onClick={() => {
                    setFolder(label as string);
                    setPage(1);
                  }}
                  className="justify-start gap-2"
                >
                  <NavIcon data-icon="inline-start" />
                  {label as string}
                  <span className="ml-auto text-xs">{count as string}</span>
                </Button>
              );
            })}
            <div className="col-span-2 mt-6 flex items-center justify-between px-2 text-xs text-muted-foreground">
              <span>文件夹</span>
              <IconButton
                label="新建文件夹"
                icon={Plus}
                size="icon-xs"
                onClick={() => demoNotice("新建文件夹")}
              />
            </div>
            {["场景参考", "服装", "环境音"].map((label) => (
              <Button
                variant="navigation"
                key={label}
                data-active={folder === label || undefined}
                onClick={() => setFolder(label)}
                className="justify-start"
              >
                <Folder data-icon="inline-start" />
                {label}
              </Button>
            ))}
          </nav>
          <div className="mt-auto hidden rounded-md bg-surface-2 p-3 text-xs lg:block">
            <div className="mb-3 flex justify-between">
              <span>本机容量</span>
              <span>[已用] / [上限]</span>
            </div>
            <Progress value={23} aria-label="本机存储容量" />
            <Button
              variant="link"
              size="xs"
              className="mt-2"
              onClick={() => demoNotice("回收站已打开")}
            >
              回收站 · 6 项
            </Button>
          </div>
        </aside>
        <div className="min-w-0 flex-1 p-4 md:p-6 lg:pl-8">
          <PageHeading title={scope === "个人" ? "我的素材" : "项目素材"}>
            <SearchInput
              value={search}
              onChange={setSearch}
              placeholder="标题、标签、备注"
              className="w-64"
            />
            <Choice
              label="素材排序"
              value={sort}
              onChange={setSort}
              options={["最近更新", "名称"]}
            />
            <Button onClick={() => uploadRef.current?.click()}>
              <Upload data-icon="inline-start" />
              上传
            </Button>
            <input
              className="sr-only"
              ref={uploadRef}
              type="file"
              multiple
              aria-label="选择素材文件"
              onChange={(e) => {
                const files = Array.from(e.target.files ?? []);
                setAssets((current) => [
                  ...files.map((file) => ({
                    name: file.name,
                    kind: file.type.startsWith("video")
                      ? "视频"
                      : file.type.startsWith("audio")
                        ? "音频"
                        : file.type.startsWith("image")
                          ? "图片"
                          : "文本",
                    ratio: "本地",
                    used: 0,
                    folder: "未分类",
                  })),
                  ...current,
                ]);
                demoNotice(`已加入 ${files.length} 个演示素材`);
              }}
            />
          </PageHeading>
          <ToggleGroup
            type="single"
            value={kind}
            onValueChange={(value) => value && setKind(value)}
            variant="pill"
            size="sm"
            aria-label="素材类型"
            className="mb-4 flex-wrap"
          >
            {["全部", "图片", "视频", "音频", "文本"].map((value) => (
              <ToggleGroupItem key={value} value={value}>
                {value}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          {uploading ? (
            <Card variant="panel" className="mb-4">
              <CardContent className="flex items-center justify-between gap-4">
                <div className="flex-1 text-xs">
                  <p className="mb-1 font-medium">上传中 · 2 / 3</p>
                  <p>dock_rain_wide.mp4</p>
                  <p className="mt-1 text-muted-foreground">lighthouse.png</p>
                </div>
                <div className="flex w-52 flex-col items-end gap-1 text-[10px] text-muted-foreground">
                  <div className="flex w-full items-center gap-3">
                    <Progress value={64} aria-label="上传进度" />
                    <span>64%</span>
                  </div>
                  <span className="text-destructive">× 格式不受支持</span>
                  <Button
                    size="xs"
                    variant="ghost"
                    onClick={() => setUploading(false)}
                  >
                    清除
                  </Button>
                </div>
              </CardContent>
            </Card>
          ) : null}
          <div
            className={cn(
              "grid items-start gap-6",
              asset ? "xl:grid-cols-[minmax(0,1fr)_280px]" : "",
            )}
          >
            <div>
              <div className="grid grid-cols-2 gap-x-6 gap-y-6 sm:grid-cols-3 xl:grid-cols-4">
                {visible.map((item) => (
                  <Card variant="project" key={item.name}>
                    <button
                      type="button"
                      aria-label={`预览${item.name}`}
                      onClick={() => setSelected(item.name)}
                      className="rounded-lg text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      <Placeholder
                        className="aspect-square"
                        kind={
                          item.kind === "视频"
                            ? "video"
                            : item.kind === "音频"
                              ? "audio"
                              : "image"
                        }
                      >
                        <span className="absolute inset-0 flex items-center justify-center bg-surface-2 font-mono text-[10px] text-subtle-foreground">
                          {item.kind === "图片"
                            ? "IMG"
                            : item.kind === "视频"
                              ? "VIDEO"
                              : item.kind === "音频"
                                ? "AUDIO"
                                : "TEXT"}
                        </span>
                        <span className="absolute right-2 bottom-2 rounded-sm bg-background px-1 font-mono text-[10px]">
                          {item.ratio}
                        </span>
                      </Placeholder>
                    </button>
                    <CardHeader className="mt-2">
                      <CardTitle>
                        <button
                          type="button"
                          onClick={() => setSelected(item.name)}
                          className="text-left"
                        >
                          {item.name}
                        </button>
                      </CardTitle>
                      <CardDescription>
                        {item.used ? `已引用 ${item.used} 处` : "未引用"}
                      </CardDescription>
                    </CardHeader>
                  </Card>
                ))}
              </div>
              {!visible.length ? <EmptySearch /> : null}
            </div>
            {asset ? (
              <Card variant="panel">
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <CardTitle>{asset.name}</CardTitle>
                    <IconButton
                      label="关闭素材预览"
                      icon={X}
                      size="icon-xs"
                      onClick={() => setSelected(null)}
                    />
                  </div>
                </CardHeader>
                <CardContent className="flex flex-col gap-4">
                  <Placeholder className="aspect-[16/9]" label="[图片预览]" />
                  <dl className="grid grid-cols-[40px_1fr] gap-x-3 gap-y-2 text-xs">
                    <dt className="text-muted-foreground">类型</dt>
                    <dd>
                      {asset.kind === "图片"
                        ? "image/png · 2048×1152"
                        : asset.kind}
                    </dd>
                    <dt className="text-muted-foreground">来源</dt>
                    <dd>本地上传</dd>
                    <dt className="text-muted-foreground">审核</dt>
                    <dd>✓ 已通过</dd>
                    <dt className="text-muted-foreground">引用</dt>
                    <dd>雾港来信 · 场景“旧码头·夜”</dd>
                  </dl>
                  <div className="flex gap-2">
                    {["雾港", "雨", "港口"].map((tag) => (
                      <Badge key={tag} variant="secondary">
                        {tag}
                      </Badge>
                    ))}
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      className="flex-1"
                      onClick={() => demoNotice("素材已加入雾港来信")}
                    >
                      加入项目
                    </Button>
                    <Button
                      variant="secondary"
                      onClick={() => {
                        setAssets((current) =>
                          current.filter((item) => item.name !== asset.name),
                        );
                        setSelected(null);
                        demoNotice("演示素材已移到回收站");
                      }}
                    >
                      移到回收站
                    </Button>
                    <IconButton
                      label={
                        favorites.includes(asset.name) ? "取消收藏" : "收藏素材"
                      }
                      icon={Star}
                      onClick={() =>
                        setFavorites((current) =>
                          current.includes(asset.name)
                            ? current.filter((name) => name !== asset.name)
                            : [...current, asset.name],
                        )
                      }
                    />
                  </div>
                </CardContent>
              </Card>
            ) : null}
          </div>
          <nav
            aria-label="素材分页"
            className="mt-10 flex items-center justify-between text-xs text-muted-foreground"
          >
            <span>共 128 项 · 每页 40</span>
            <div className="flex gap-2">
              {[1, 2, 3, 4].map((value) => (
                <Button
                  key={value}
                  variant={page === value ? "secondary" : "ghost"}
                  size="icon-xs"
                  aria-label={`第 ${value} 页`}
                  onClick={() => {
                    setPage(value);
                    demoNotice(`第 ${value} 页`);
                  }}
                >
                  {value}
                </Button>
              ))}
            </div>
          </nav>
        </div>
      </div>
    </ProductShell>
  );
}
