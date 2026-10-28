"use client";
import Image from "next/image";
import { useState, type ReactNode } from "react";
import { Search } from "lucide-react";
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
import { ContentState } from "@/components/layout/content-state";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  DataPagination,
  type PageSize,
} from "@/components/media/data-pagination";
const homeCategories = [
  "精选",
  "广告营销",
  "影视作品",
  "平面设计",
  "电商",
  "动画动漫",
];
const works = [
  {
    src: "cobalt-headphones.png",
    title: "声色之间 · 产品概念",
    categories: ["广告营销", "平面设计"],
    scene: "品牌广告",
  },
  {
    src: "moonlight-whale.png",
    title: "云海鲸歌 · 动画概念",
    categories: ["广告营销", "影视作品", "动画动漫"],
    scene: "品牌广告",
  },
  {
    src: "amber-perfume.png",
    title: "琥珀香氛 · 暖金时刻",
    categories: ["广告营销", "电商"],
    scene: "品牌广告",
  },
  {
    src: "alpine-road.png",
    title: "山野公路 · 晨光旅程",
    categories: ["广告营销", "影视作品"],
    scene: "品牌广告",
  },
  {
    src: "cobalt-headphones.png",
    title: "声色之间 · 耳机广告",
    categories: ["广告营销", "平面设计"],
    scene: "社媒营销",
  },
  {
    src: "moonlight-whale.png",
    title: "云海鲸歌 · 色彩参考",
    categories: ["广告营销", "影视作品", "动画动漫"],
    scene: "社媒营销",
  },
  {
    src: "amber-perfume.png",
    title: "琥珀香氛 · 日落篇",
    categories: ["广告营销", "电商"],
    scene: "社媒营销",
  },
  {
    src: "citrus-still-life.png",
    title: "夏日柑橘 · 静物广告",
    categories: ["广告营销", "影视作品"],
    scene: "社媒营销",
  },
];
const homeWorks = [
  { ...works[0], title: "声色之间的产品灵感" },
  { ...works[2], title: "夏日香氛 · 光的形状" },
  { ...works[1], title: "想象之外，自有故事" },
  { ...works[3], title: "山野公路 · 晨光旅程" },
];

type Preview = { src: string; title: string };
function InspirationPreview({
  item,
  children,
}: {
  item: Preview;
  children: ReactNode;
}) {
  return (
    <Dialog>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent className="sm:max-w-3xl">
        <DialogTitle>{item.title}</DialogTitle>
        <DialogDescription>
          AI 生成的静态参考图，探索构图、材质与光线。
        </DialogDescription>
        <div className="relative aspect-video">
          <Image
            src={`/design/reference/${item.src}`}
            alt={item.title}
            fill
            sizes="80vw"
            className="object-contain"
          />
        </div>
      </DialogContent>
    </Dialog>
  );
}
export function InspirationGallery({ searchQuery }: { searchQuery?: string }) {
  const [category, setCategory] = useState("精选");
  const [localQuery, setQuery] = useState("");
  const query = searchQuery ?? localQuery;
  const [pageSize, setPageSize] = useState<PageSize>(12);
  const [pagination, setPagination] = useState({ filter: "", page: 1 });
  const filterKey = JSON.stringify([category, query]);
  const matches = homeWorks.filter(
    (w) =>
      (category === "精选" ||
        works.some(
          (item) => item.src === w.src && item.categories.includes(category),
        )) &&
      w.title.includes(query.trim()),
  );
  const pages = Math.max(1, Math.ceil(matches.length / pageSize));
  const page = Math.min(
    pages,
    pagination.filter === filterKey ? pagination.page : 1,
  );
  const visible = matches.slice((page - 1) * pageSize, page * pageSize);
  return (
    <section className="inspiration">
      <h2>创作精选</h2>
      <Tabs value={category} onValueChange={setCategory} className="gap-0">
        <div className="gallery-toolbar">
          <div className="gallery-categories">
            <TabsList variant="navigation">
              {homeCategories.map((c) => (
                <TabsTrigger key={c} value={c}>
                  {c}
                </TabsTrigger>
              ))}
            </TabsList>
          </div>
          {searchQuery === undefined && (
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
        <TabsContent value={category}>
          <h2 className="gallery-heading">
            {category === "精选" ? "广告与视觉创意" : category}
          </h2>
          <ReferenceGrid items={visible} />
          {!matches.length && (
            <ContentState
              title="没有匹配的灵感"
              description="试试其他分类或关键词。"
            />
          )}
          <p className="source-caption">AI 生成的创作参考</p>
          <DataPagination
            label="灵感分页"
            page={page}
            pageSize={pageSize}
            total={matches.length}
            onPageChange={(page) => setPagination({ filter: filterKey, page })}
            onPageSizeChange={(size) => {
              setPageSize(size);
              setPagination({ filter: filterKey, page: 1 });
            }}
          />
        </TabsContent>
      </Tabs>
    </section>
  );
}

function ReferenceGrid({ items }: { items: Preview[] }) {
  return (
    <div className="inspiration-grid">
      {items.map((w) => (
        <InspirationPreview key={w.title} item={w}>
          <Button
            variant="ghost"
            className="work-entry h-auto p-0 whitespace-normal"
          >
            <Card variant="media" className="w-full">
              <CardContent>
                <div className="work-cover">
                  <Image
                    src={`/design/reference/${w.src}`}
                    alt={w.title}
                    fill
                    sizes={"(max-width: 767px) 45vw, 33vw"}
                  />
                </div>
              </CardContent>
              <CardHeader>
                <CardTitle>{w.title}</CardTitle>
                <CardDescription>AI 生成示例</CardDescription>
              </CardHeader>
            </Card>
          </Button>
        </InspirationPreview>
      ))}
    </div>
  );
}

export function ExploreReferenceExamples({
  scene,
  query,
}: {
  scene: string;
  query: string;
}) {
  const matches = works.filter(
    (item) => item.scene === scene && item.title.includes(query.trim()),
  );
  return (
    <section
      className="inspiration explore-gallery"
      aria-label="广告营销示例作品"
    >
      <ReferenceGrid items={matches} />
      {!matches.length && (
        <ContentState
          title="没有匹配的示例作品"
          description="试试其他关键词或场景。"
        />
      )}
      <p className="source-caption">AI 生成的静态示例，仅供创作参考。</p>
    </section>
  );
}
