"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Search as SearchIcon, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";

type Result = { url: string; meta: { title?: string }; excerpt: string };
type Pagefind = {
  search: (
    query: string,
    options: { filters?: Record<string, string> },
  ) => Promise<{ results: { data: () => Promise<Result> }[] }>;
};
export function Search() {
  const params = useSearchParams();
  const [query, setQuery] = useState(params.get("q") ?? "");
  const [category, setCategory] = useState("");
  const [retry, setRetry] = useState(0);
  const [results, setResults] = useState<Result[]>([]);
  const [state, setState] = useState<"idle" | "loading" | "ready" | "error">(
    "idle",
  );
  useEffect(() => {
    if (!query.trim()) return;
    let canceled = false;
    const timer = setTimeout(async () => {
      setState("loading");
      try {
        // The build emits Pagefind assets. Variable import deliberately stays a browser URL.
        const url = `${process.env.NEXT_PUBLIC_WORKSPACE_BASE_PATH ?? ""}/pagefind/pagefind.js`;
        const pagefind = (await import(
          /* webpackIgnore: true */ url
        )) as Pagefind;
        const found = await pagefind.search(query, {
          filters: category ? { category } : undefined,
        });
        const data = await Promise.all(
          found.results.slice(0, 30).map((r) => r.data()),
        );
        if (!canceled) {
          setResults(data);
          setState("ready");
        }
      } catch {
        if (!canceled) setState("error");
      }
    }, 180);
    return () => {
      canceled = true;
      clearTimeout(timer);
    };
  }, [query, category, retry]);
  const activeState = query.trim() ? state : "idle";
  return (
    <>
      <div className="search-box">
        <Input
          type="search"
          aria-label="全文搜索"
          placeholder="搜索中文关键词、FR 或 SRC 编号…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          autoFocus
        />
        <Button
          aria-label="重新搜索"
          variant="outline"
          onClick={() => setRetry((n) => n + 1)}
        >
          <SearchIcon data-icon="inline-start" />
          搜索
        </Button>
      </div>
      <div className="search-filters">
        <label htmlFor="search-category">文档分类</label>
        <select
          id="search-category"
          value={category}
          onChange={(e) => setCategory(e.target.value)}
        >
          <option value="">全部分类</option>
          {[
            "原始需求",
            "设计方案",
            "产品需求",
            "执行计划",
            "调研资料",
            "文档模板",
            "Skill",
            "工作流",
          ].map((name) => (
            <option key={name}>{name}</option>
          ))}
        </select>
        {activeState === "ready" && (
          <span aria-live="polite">显示 {results.length} 条结果</span>
        )}
      </div>
      {activeState === "idle" && (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>从一个关键词开始</EmptyTitle>
            <EmptyDescription>支持中文、文档标题和业务编号。</EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      {activeState === "loading" && (
        <p className="status-message" role="status">
          正在搜索当前知识版本…
        </p>
      )}
      {activeState === "error" && (
        <Alert>
          <AlertTitle>搜索索引暂时不可用</AlertTitle>
          <AlertDescription>
            请检查资源是否完整，或重新加载索引。
            <Button
              variant="outline"
              size="sm"
              onClick={() => setRetry((n) => n + 1)}
            >
              <RefreshCw data-icon="inline-start" />
              重试
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {activeState === "ready" && !results.length && (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>没有找到相关文档</EmptyTitle>
            <EmptyDescription>
              试试更短的关键词，或选择全部分类。
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      {activeState === "ready" &&
        results.map((r) => (
          <article key={r.url} className="search-result">
            <Link href={r.url}>
              <h2>{r.meta.title ?? r.url}</h2>
            </Link>
            <p dangerouslySetInnerHTML={{ __html: r.excerpt }} />
          </article>
        ))}
    </>
  );
}
