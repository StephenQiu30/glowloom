"use client";

import { useRef, useState } from "react";
import { cn } from "cn";
import {
  Activity,
  Check,
  Clock3,
  Folder,
  Grid2X2,
  Plus,
  ShieldAlert,
  Star,
  Upload,
  UserRound,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Progress } from "@/components/ui/progress";

import {
  PreviewShell,
  PageHeading,
  SearchInput,
  Choice,
  IconButton,
  LocalDialog,
  Placeholder,
  demoNotice,
  EmptySearch,
} from "./shared";

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
export function AssetsPreviewPage() {
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
    <PreviewShell screen="assets" fullBleed>
      <div className="flex min-h-svh flex-col lg:flex-row">
        <aside className="flex w-full shrink-0 flex-col gap-6 bg-surface-1 p-5 lg:sticky lg:top-0 lg:h-svh lg:w-[260px]">
          <ToggleGroup
            type="single"
            value={scope}
            onValueChange={(value) => value && setScope(value)}
            aria-label="素材范围"
            className="w-full"
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
        <div className="min-w-0 flex-1 p-4 md:p-6">
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
            <Card variant="panel" className="mb-5">
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
              "grid items-start gap-5",
              asset ? "xl:grid-cols-[minmax(0,1fr)_280px]" : "",
            )}
          >
            <div>
              <div className="grid grid-cols-2 gap-x-5 gap-y-6 sm:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-4">
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
          <footer className="mt-10 flex items-center justify-between text-xs text-muted-foreground">
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
          </footer>
        </div>
      </div>
    </PreviewShell>
  );
}
const bibleEntries = {
  角色: ["林舟", "沈岚", "老船主", "周警官", "阿梨", "船员"],
  场景: ["旧码头 · 夜", "灯塔", "林舟的家", "渔船甲板"],
  道具: [
    "旧信封",
    "雨伞",
    "录音机",
    "怀表",
    "船票",
    "手电",
    "钥匙",
    "照片",
    "指南针",
  ],
};
export function BiblePreviewPage() {
  const [category, setCategory] = useState<keyof typeof bibleEntries>("角色");
  const [search, setSearch] = useState("");
  const [unconfirmed, setUnconfirmed] = useState(false);
  const [selected, setSelected] = useState("林舟");
  const [look, setLook] = useState("雨夜造型 · v2");
  const [dialog, setDialog] = useState<string | null>(null);
  const [newName, setNewName] = useState("");
  const [extra, setExtra] = useState<string[]>([]);
  const [generated, setGenerated] = useState(false);
  const [playing, setPlaying] = useState(false);
  const visible = [...bibleEntries[category], ...extra].filter(
    (name, index) =>
      name.includes(search) && (!unconfirmed || ![0, 1, 4].includes(index)),
  );
  return (
    <PreviewShell screen="bible" contextual>
      <PageHeading
        title="设定集"
        description="从剧本提取的角色、场景与道具；变更版本独立管理。"
      >
        <Button variant="secondary" onClick={() => setDialog("合并别名")}>
          合并别名
        </Button>
        <Button onClick={() => setDialog("新建条目")}>
          <Plus data-icon="inline-start" />
          新建条目
        </Button>
      </PageHeading>
      <div className="mb-5 flex flex-wrap items-center justify-between gap-4">
        <ToggleGroup
          type="single"
          value={category}
          onValueChange={(value) => {
            if (value) {
              const cat = value as keyof typeof bibleEntries;
              setCategory(cat);
              setSelected(bibleEntries[cat][0]);
              setExtra([]);
            }
          }}
          aria-label="设定类型"
        >
          {Object.entries(bibleEntries).map(([name, entries]) => (
            <ToggleGroupItem key={name} value={name}>
              {name} {entries.length}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <div className="flex flex-wrap items-center gap-4">
          <Field orientation="horizontal" className="w-auto">
            <Switch
              id="unconfirmed"
              checked={unconfirmed}
              onCheckedChange={setUnconfirmed}
            />
            <FieldLabel htmlFor="unconfirmed">只看未确认</FieldLabel>
          </Field>
          <SearchInput
            value={search}
            onChange={setSearch}
            placeholder="搜索名称或别名"
            className="w-60"
          />
        </div>
      </div>
      <div className="grid items-start gap-5 lg:grid-cols-[260px_minmax(0,1fr)]">
        <nav className="flex flex-col gap-2" aria-label="设定条目">
          {visible.map((name, index) => (
            <button
              type="button"
              key={name}
              onClick={() => {
                setSelected(name);
                setGenerated(false);
              }}
              aria-current={name === selected ? "true" : undefined}
              className={cn(
                "flex items-center gap-3 rounded-lg p-3 text-left outline-none hover:bg-surface-2 focus-visible:ring-2 focus-visible:ring-ring",
                name === selected && "bg-surface-3",
              )}
            >
              <span className="flex size-12 shrink-0 items-center justify-center rounded-md bg-surface-2">
                <UserRound className="size-4 text-subtle-foreground" />
              </span>
              <span className="flex-1 text-sm font-medium">
                {name}
                <span className="mt-1 block text-[10px] font-normal text-muted-foreground">
                  出场 {index < 2 ? 5 : 2} 集 · {index < 2 ? 2 : 1} 个造型
                </span>
              </span>
              <span className="text-[10px] text-muted-foreground">
                {index < 2 || index === 4
                  ? "已锁定"
                  : index === 2
                    ? "待定"
                    : "未生成"}
              </span>
            </button>
          ))}
          {!visible.length ? <EmptySearch /> : null}
        </nav>
        <Card variant="panel">
          <CardHeader>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <CardTitle>{selected}</CardTitle>
                <CardDescription>
                  别名：{selected === "林舟" ? "小舟、林先生" : selected} ·
                  出场第 1–5 集
                </CardDescription>
              </div>
              <div className="flex gap-2">
                <Button variant="outline" onClick={() => setDialog("上传参考")}>
                  上传参考
                </Button>
                <Button
                  onClick={() => {
                    setGenerated(true);
                    demoNotice("参考定帧位已生成");
                  }}
                >
                  生成定帧位
                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent className="flex flex-col gap-5">
            <div className="flex items-center gap-2">
              <ToggleGroup
                type="single"
                value={look}
                onValueChange={(value) => value && setLook(value)}
                aria-label="角色造型"
              >
                {["日常", "雨夜造型 · v2", "回忆"].map((value) => (
                  <ToggleGroupItem key={value} value={value}>
                    {value === "雨夜造型 · v2" ? <Check /> : null}
                    {value}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
              <IconButton
                label="新建造型"
                icon={Plus}
                onClick={() => setDialog("新建造型")}
              />
            </div>
            <p className="text-xs text-muted-foreground">参考定帧位</p>
            <div className="grid grid-cols-2 gap-4 xl:grid-cols-4">
              {["正面半身", "侧面", "全身", "表情组"].map((label, index) => (
                <div key={label}>
                  <div className="relative">
                    <Placeholder className="aspect-[3/4]">
                      {index < 2 || generated ? (
                        <Badge className="absolute top-2 left-2">
                          <Check />
                          已锁定
                        </Badge>
                      ) : (
                        <span className="absolute bottom-1/3 text-xs text-muted-foreground">
                          未生成
                        </span>
                      )}
                    </Placeholder>
                  </div>
                  <div className="mt-2 flex items-center justify-between text-xs">
                    <span>{label}</span>
                    {index === 2 ? <Badge variant="muted">候选 4</Badge> : null}
                  </div>
                </div>
              ))}
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <Card>
                <CardHeader>
                  <CardTitle>音色</CardTitle>
                </CardHeader>
                <CardContent>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setPlaying(!playing);
                      demoNotice(playing ? "试听已暂停" : "音色试听演示");
                    }}
                  >
                    <Activity data-icon="inline-start" />
                    {playing ? "暂停试听" : "[音色名称]"}
                  </Button>
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <CardTitle>真人授权</CardTitle>
                    <Badge variant="warning">
                      <ShieldAlert />
                      待声明
                    </Badge>
                  </div>
                </CardHeader>
                <CardContent>
                  <p className="text-[11px] leading-5 text-muted-foreground">
                    上传的声音来自真人时，需完成授权声明才能进入生成队列。
                  </p>
                  <Button
                    variant="link"
                    size="xs"
                    onClick={() => setDialog("真人授权声明")}
                  >
                    填写声明
                  </Button>
                </CardContent>
              </Card>
            </div>
          </CardContent>
        </Card>
      </div>
      <LocalDialog
        open={dialog !== null}
        onOpenChange={(open) => !open && setDialog(null)}
        title={dialog ?? "编辑设定"}
        onConfirm={() => {
          if (dialog === "新建条目" && newName.trim()) {
            setExtra([...extra, newName]);
            setSelected(newName);
            setNewName("");
          }
          demoNotice("设定已更新");
        }}
      >
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="bible-edit">
              {dialog === "合并别名" ? "别名" : "名称或说明"}
            </FieldLabel>
            <Input
              id="bible-edit"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder={selected}
            />
          </Field>
          {dialog === "真人授权声明" ? (
            <Field>
              <FieldLabel htmlFor="permission-description">授权说明</FieldLabel>
              <Textarea
                id="permission-description"
                placeholder="本地演示，无真实授权提交"
              />
            </Field>
          ) : null}
        </FieldGroup>
      </LocalDialog>
    </PreviewShell>
  );
}
