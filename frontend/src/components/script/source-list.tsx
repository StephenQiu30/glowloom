"use client";

import { useDeferredValue, useId, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { SourceSummary } from "./source-model";

export function SourceList({
  items,
  selected,
  locked,
  nextPage,
  loadingNext,
  pageError,
  onSelect,
  onNext,
}: {
  items: SourceSummary[];
  selected?: string;
  locked: boolean;
  nextPage: boolean;
  loadingNext: boolean;
  pageError?: string;
  onSelect: (lineageId: string) => void;
  onNext: () => void;
}) {
  "use no memo"; // TanStack Virtual 3.14 的可变实例方法当前不兼容 React Compiler。
  const [search, setSearch] = useState("");
  const id = useId();
  const query = useDeferredValue(search.trim().toLocaleLowerCase("zh-CN"));
  const filtered = query
    ? items.filter((item) =>
        item.title.toLocaleLowerCase("zh-CN").includes(query),
      )
    : items;
  const viewport = useRef<HTMLDivElement>(null);
  // 此组件通过上方 use no memo 显式退出 Compiler；保留 Virtual 的实际滚动测量。
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtual = useVirtualizer({
    count: filtered.length,
    getScrollElement: () => viewport.current,
    estimateSize: () => 76,
    overscan: 8,
    enabled: filtered.length > 100,
  });
  function row(item: SourceSummary) {
    return (
      <Button
        type="button"
        variant={selected === item.source_lineage_id ? "secondary" : "ghost"}
        className="h-auto min-h-[72px] w-full min-w-0 flex-col items-start gap-1 text-left whitespace-normal"
        disabled={locked}
        aria-label={`打开来源 ${item.title}`}
        aria-current={selected === item.source_lineage_id ? "true" : undefined}
        onClick={() => onSelect(item.source_lineage_id)}
      >
        <span className="line-clamp-2 w-full break-words">
          {item.position + 1}. {item.title}
        </span>
        <span className="text-xs text-muted-foreground">
          {item.source_kind === "chapter"
            ? "章节"
            : item.source_kind === "episode"
              ? "来源集"
              : "原文"}{" "}
          ·{" "}
          {item.status === "draft"
            ? "草稿"
            : item.status === "ready"
              ? "可用"
              : "完成"}{" "}
          · {item.char_count.toLocaleString("zh-CN")} 字符
        </span>
      </Button>
    );
  }
  return (
    <div className="min-w-0 space-y-3">
      <Label htmlFor={id}>搜索已读取来源</Label>
      <Input
        id={id}
        value={search}
        disabled={locked}
        onChange={(event) => setSearch(event.target.value)}
      />
      <p className="text-sm text-muted-foreground">
        已读取 {items.length.toLocaleString("zh-CN")} 个来源摘要
        {nextPage ? "，仍有下一页" : ""}
      </p>
      <div
        ref={viewport}
        className="max-h-[min(60dvh,560px)] overflow-y-auto"
        tabIndex={filtered.length > 100 ? 0 : undefined}
        aria-label={filtered.length > 100 ? "来源列表滚动区" : undefined}
      >
        {!filtered.length ? (
          <p className="py-4 text-sm">已读取的来源中没有匹配项。</p>
        ) : filtered.length > 100 ? (
          <div
            role="list"
            style={{ height: virtual.getTotalSize(), position: "relative" }}
          >
            {virtual.getVirtualItems().map((entry) => (
              <div
                key={filtered[entry.index].id}
                role="listitem"
                data-index={entry.index}
                ref={virtual.measureElement}
                style={{
                  position: "absolute",
                  top: 0,
                  left: 0,
                  width: "100%",
                  transform: `translateY(${entry.start}px)`,
                }}
              >
                {row(filtered[entry.index])}
              </div>
            ))}
          </div>
        ) : (
          <ul className="space-y-1">
            {filtered.map((item) => (
              <li key={item.id}>{row(item)}</li>
            ))}
          </ul>
        )}
      </div>
      {pageError && (
        <p role="alert" className="text-sm text-destructive">
          {pageError}
        </p>
      )}
      {(nextPage || pageError) && (
        <Button
          type="button"
          variant="outline"
          disabled={loadingNext || locked}
          onClick={onNext}
        >
          {loadingNext
            ? "正在读取下一页…"
            : pageError
              ? "重试读取下一页"
              : "读取下一页"}
        </Button>
      )}
    </div>
  );
}
