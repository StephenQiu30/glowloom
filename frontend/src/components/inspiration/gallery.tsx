"use client";
import Image from "next/image";
import Link from "next/link";
import { useState } from "react";
import { ArrowUpRight, Search } from "lucide-react";
import { cn } from "cn";
import { BrandMark } from "@/components/brand/brand";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import {
  InputGroup,
  InputGroupInput,
  InputGroupAddon,
} from "@/components/ui/input-group";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { UnavailableDialog } from "@/components/availability/unavailable";
const homeCategories = [
  "精选",
  "广告营销",
  "影视作品",
  "平面设计",
  "电商",
  "动画动漫",
];
const exploreCategories = [
  "广告营销",
  "影视作品",
  "平面设计",
  "电商",
  "动画动漫",
  "其他",
  "技能",
  "活动",
];
const works = [
  {
    src: "butterfly.jpg",
    title: "蓝蝶 · 微观光影",
    categories: ["广告营销", "平面设计"],
    scene: "品牌广告",
    author: "织光创作团队",
    duration: "00:48",
  },
  {
    src: "fantasy.jpg",
    title: "幻想短片 · 动作概念",
    categories: ["广告营销", "影视作品", "动画动漫"],
    scene: "品牌广告",
    author: "视觉实验室",
    duration: "00:32",
  },
  {
    src: "perfume.png",
    title: "海边香氛 · 暖金时刻",
    categories: ["广告营销", "电商"],
    scene: "品牌广告",
    author: "织光创作团队",
    duration: "00:50",
  },
  {
    src: "sunrise.jpg",
    title: "她让太阳升起",
    categories: ["广告营销", "影视作品"],
    scene: "品牌广告",
    author: "视觉实验室",
    duration: "01:20",
  },
  {
    src: "butterfly.jpg",
    title: "微观世界 · 蓝色记忆",
    categories: ["广告营销", "平面设计"],
    scene: "社媒营销",
    author: "织光创作团队",
    duration: "00:48",
  },
  {
    src: "fantasy.jpg",
    title: "光影实验 · 未来片场",
    categories: ["广告营销", "影视作品", "动画动漫"],
    scene: "社媒营销",
    author: "视觉实验室",
    duration: "00:32",
  },
  {
    src: "perfume.png",
    title: "香氛广告 · 日落篇",
    categories: ["广告营销", "电商"],
    scene: "社媒营销",
    author: "织光创作团队",
    duration: "00:50",
  },
  {
    src: "sunrise.jpg",
    title: "夏日 · 视觉叙事",
    categories: ["广告营销", "影视作品"],
    scene: "社媒营销",
    author: "视觉实验室",
    duration: "01:20",
  },
];
const homeWorks = [
  { src: "butterfly.jpg", title: "微观世界的奇妙旅程" },
  { src: "sunrise.jpg", title: "夏日香氛 · 光的形状" },
  { src: "fantasy.jpg", title: "让想象成为镜头" },
];
const updates = [
  { src: "perfume.png", title: "光影之间", caption: "精选创作 · 夏日香氛" },
  {
    src: "butterfly.jpg",
    title: "微观世界",
    caption: "画布灵感 · 探索另一种尺度",
  },
  {
    src: "fantasy.jpg",
    title: "让想象成为镜头",
    caption: "叙事短片 · 从分镜开始",
  },
];
type Preview = { src: string; title: string };
export function InspirationGallery({
  showUpdates = true,
}: {
  showUpdates?: boolean;
}) {
  const [category, setCategory] = useState(showUpdates ? "精选" : "广告营销");
  const [scene, setScene] = useState("品牌广告");
  const [query, setQuery] = useState("");
  const [preview, setPreview] = useState<Preview | null>(null);
  const [publish, setPublish] = useState(false);
  const matches = showUpdates
    ? homeWorks.filter(
        (w) =>
          (category === "精选" ||
            works.some(
              (item) =>
                item.src === w.src && item.categories.includes(category),
            )) &&
          w.title.includes(query.trim()),
      )
    : works.filter(
        (w) =>
          w.categories.includes(category) &&
          w.title.includes(query.trim()) &&
          (scene === "品牌广告" || w.scene === scene),
      );
  return (
    <section className={cn("inspiration", !showUpdates && "explore-gallery")}>
      {showUpdates && (
        <>
          <h2>最近上新</h2>
          <div className="updates-strip" aria-label="最近上新">
            {updates.map((item) => (
              <button
                key={item.title}
                className="feature-entry"
                onClick={() => setPreview(item)}
              >
                <Card variant="feature">
                  <CardContent>
                    <Image
                      src={`/design/figma/${item.src}`}
                      alt={item.title}
                      fill
                      sizes="(max-width: 767px) 308px, (min-width: 1700px) 25vw, 273px"
                    />
                  </CardContent>
                  <CardHeader>
                    <CardTitle>{item.title}</CardTitle>
                    <CardDescription>{item.caption}</CardDescription>
                  </CardHeader>
                </Card>
              </button>
            ))}
            <Link href="/projects" className="feature-entry">
              <Card variant="feature" data-feature="start">
                <CardContent>
                  <BrandMark monochrome />
                </CardContent>
                <CardHeader>
                  <CardTitle>从灵感，到作品</CardTitle>
                  <CardDescription>织光 Glowloom · 开启创作</CardDescription>
                </CardHeader>
              </Card>
            </Link>
          </div>
        </>
      )}
      <div className="gallery-toolbar">
        <Tabs
          value={category}
          onValueChange={setCategory}
          className="gallery-categories"
        >
          <TabsList variant="navigation">
            {(showUpdates ? homeCategories : exploreCategories).map((c) => (
              <TabsTrigger key={c} value={c}>
                {c}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        {showUpdates && (
          <InputGroup className="gallery-search">
            <InputGroupAddon>
              <Search />
            </InputGroupAddon>
            <InputGroupInput
              aria-label="搜索灵感"
              placeholder="搜索灵感、作品或创作者"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </InputGroup>
        )}
      </div>
      {!showUpdates && (
        <div className="explore-toolbar">
          <ToggleGroup
            type="single"
            value={scene}
            onValueChange={(v) => {
              if (v) setScene(v);
            }}
            aria-label="作品场景"
          >
            <ToggleGroupItem value="品牌广告">品牌广告</ToggleGroupItem>
            <ToggleGroupItem value="社媒营销">社媒营销</ToggleGroupItem>
          </ToggleGroup>
          <InputGroup className="explore-search">
            <InputGroupAddon>
              <Search />
            </InputGroupAddon>
            <InputGroupInput
              aria-label="搜索作品"
              placeholder="搜索作品"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </InputGroup>
          <Button size="lg" onClick={() => setPublish(true)}>
            发布短片
          </Button>
        </div>
      )}
      {showUpdates && (
        <h2 className="gallery-heading">
          {category === "精选" ? "广告与视觉创意" : category}
        </h2>
      )}
      <div className="inspiration-grid">
        {matches.map((w) => (
          <button
            key={w.title}
            className="work-entry"
            onClick={() => setPreview(w)}
          >
            <Card variant="media">
              <CardContent>
                <div className="work-cover">
                  <Image
                    src={`/design/figma/${w.src}`}
                    alt={w.title}
                    fill
                    sizes={
                      showUpdates
                        ? "(max-width: 767px) 45vw, 33vw"
                        : "(max-width: 767px) 45vw, 25vw"
                    }
                  />
                </div>
              </CardContent>
              <CardHeader>
                <CardTitle>{w.title}</CardTitle>
                <CardDescription>
                  {showUpdates ? (
                    <>
                      <span>织光精选 · 查看创作过程</span>
                      <ArrowUpRight aria-hidden="true" data-icon="inline-end" />
                    </>
                  ) : (
                    `${"author" in w ? w.author : "织光精选"} · ${"duration" in w ? w.duration : ""}`
                  )}
                </CardDescription>
              </CardHeader>
            </Card>
          </button>
        ))}
      </div>
      {!matches.length && (
        <Empty className="min-h-72">
          <EmptyHeader>
            <EmptyTitle>没有匹配的灵感</EmptyTitle>
            <EmptyDescription>试试其他分类或关键词。</EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      <p className="source-caption">
        {showUpdates
          ? "设计稿灵感示例 · 封面仅供浏览参考"
          : "发现好作品，把灵感带回自己的画布。设计稿展示内容，发布功能尚未接入。"}
      </p>
      <Dialog
        open={!!preview}
        onOpenChange={(value) => {
          if (!value) setPreview(null);
        }}
      >
        <DialogContent className="sm:max-w-3xl">
          <DialogTitle>{preview?.title}</DialogTitle>
          <DialogDescription>
            设计稿灵感示例。封面可浏览，作品发布与创作过程尚未接入。
          </DialogDescription>
          {preview && (
            <div className="relative aspect-video">
              <Image
                src={`/design/figma/${preview.src}`}
                alt={preview.title}
                fill
                sizes="80vw"
                className="object-contain"
              />
            </div>
          )}
        </DialogContent>
      </Dialog>
      <UnavailableDialog
        feature={publish ? "发布短片" : null}
        onClose={() => setPublish(false)}
      />
    </section>
  );
}
