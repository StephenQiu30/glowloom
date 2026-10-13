"use client";
import Image from "next/image";
import { useState } from "react";
import { Search } from "lucide-react";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
const categories = [
  "精选",
  "广告营销",
  "影视作品",
  "平面设计",
  "电商",
  "动画动漫",
];
const works = [
  {
    src: "butterfly-a86a055c.jpeg",
    title: "微观世界 · 蓝色记忆",
    category: "平面设计",
  },
  { src: "cinema-9f1637c2.jpeg", title: "光影叙事", category: "广告营销" },
  {
    src: "fantasy-cc4ea671.jpeg",
    title: "幻想短片 · 动作概念",
    category: "影视作品",
  },
];
export function InspirationGallery({
  showUpdates = true,
}: {
  showUpdates?: boolean;
}) {
  const [category, setCategory] = useState("精选");
  const [query, setQuery] = useState("");
  const [preview, setPreview] = useState<{ src: string; title: string } | null>(
    null,
  );
  const matches = works.filter(
    (w) =>
      (category === "精选" || w.category === category) &&
      w.title.includes(query.trim()),
  );
  return (
    <section className="inspiration">
      {showUpdates && (
        <>
          <h2>最近上新</h2>
          <div className="updates-strip" aria-label="最近上新">
            {[
              { src: "mango-51cf3732.jpeg", title: "精选作品 MANGO GLASS PRO" },
              { src: "cli-69571eae.jpeg", title: "即梦 CLI" },
              { src: "seedance-3ab44a01.jpeg", title: "Seedance 2.5 样片模式" },
              { src: "studio-ee1a78d9.png", title: "即梦片场上线" },
            ].map((item) => (
              <button
                key={item.src}
                onClick={() => setPreview(item)}
                aria-label={item.title}
              >
                <Image
                  src={`/design/${item.src}`}
                  alt={item.title}
                  fill
                  sizes="320px"
                />
              </button>
            ))}
          </div>
        </>
      )}
      <div className="gallery-toolbar">
        <Tabs value={category} onValueChange={setCategory}>
          <TabsList className="bg-transparent">
            {categories.map((c) => (
              <TabsTrigger key={c} value={c} className="rounded-full">
                {c}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <label className="gallery-search">
          <Search size={16} />
          <span className="sr-only">搜索灵感</span>
          <Input
            placeholder="搜索创作灵感"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
      </div>
      <h2>{category === "精选" ? "创作灵感" : category}</h2>
      <div className="inspiration-grid">
        {matches.map((w) => (
          <button key={w.src} onClick={() => setPreview(w)}>
            <div>
              <Image
                src={`/design/${w.src}`}
                alt={w.title}
                fill
                sizes="(max-width: 700px) 90vw, 33vw"
              />
            </div>
            <span>{w.title}</span>
          </button>
        ))}
      </div>
      {!matches.length && (
        <p className="empty-state">没有匹配的灵感，试试其他分类或关键词。</p>
      )}
      <p className="source-caption">
        视觉参考来自即梦公开作品，仅供创作灵感浏览。
      </p>
      <Dialog
        open={!!preview}
        onOpenChange={(open) => {
          if (!open) setPreview(null);
        }}
      >
        <DialogContent className="sm:max-w-3xl">
          <DialogTitle>{preview?.title}</DialogTitle>
          <DialogDescription>即梦公开视觉参考</DialogDescription>
          {preview && (
            <div className="relative aspect-video">
              <Image
                src={`/design/${preview.src}`}
                alt={preview.title}
                fill
                sizes="80vw"
                className="object-contain"
              />
            </div>
          )}
        </DialogContent>
      </Dialog>
    </section>
  );
}
