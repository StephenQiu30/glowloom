"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { listProjects } from "./generated/projects";
import {
  requireProjectList,
  type Project,
  projectErrorMessage,
} from "./project";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
export function ProjectPicker({
  value,
  onChange,
  disabled = false,
}: {
  value: Project | null;
  onChange: (p: Project) => void;
  disabled?: boolean;
}) {
  const [items, setItems] = useState<Project[]>([]),
    [search, setSearch] = useState(""),
    [query, setQuery] = useState(""),
    [cursor, setCursor] = useState<string | null>(null),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [retry, setRetry] = useState(0);
  useEffect(() => {
    const c = new AbortController();
    listProjects({ q: query, limit: 50 }, { signal: c.signal })
      .then(requireProjectList)
      .then((p) => {
        if (!c.signal.aborted) {
          setItems(p.items);
          setCursor(p.next_cursor);
          setError("");
        }
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(projectErrorMessage(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [query, retry]);
  async function more() {
    if (!cursor || loading) return;
    setLoading(true);
    try {
      const p = requireProjectList(
        await listProjects({ q: query, limit: 50, cursor }),
      );
      setItems((i) => [
        ...i,
        ...p.items.filter((v) => !i.some((a) => a.id === v.id)),
      ]);
      setCursor(p.next_cursor);
    } catch (e) {
      setError(projectErrorMessage(e));
    } finally {
      setLoading(false);
    }
  }
  return (
    <div className="project-picker">
      <label>
        所属项目
        <select
          aria-label="所属项目"
          disabled={disabled || loading}
          value={value?.id ?? ""}
          onChange={(e) => {
            const p = items.find((p) => p.id === e.target.value);
            if (p) onChange(p);
          }}
        >
          <option value="">{loading ? "正在读取项目…" : "选择项目"}</option>
          {value && !items.some((p) => p.id === value.id) && (
            <option value={value.id}>{value.name}</option>
          )}
          {items.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
      </label>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          setLoading(true);
          setQuery(search.trim());
          setRetry((n) => n + 1);
        }}
      >
        <Input
          aria-label="查找项目"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="查找项目"
          disabled={disabled}
          maxLength={100}
        />
        <Button variant="ghost" disabled={disabled || loading}>
          查找
        </Button>
      </form>
      {cursor && (
        <Button
          variant="ghost"
          disabled={disabled || loading}
          onClick={() => void more()}
        >
          更多项目
        </Button>
      )}
      {error ? (
        <div role="alert">
          {error}
          <Button
            variant="ghost"
            onClick={() => {
              setLoading(true);
              setRetry((n) => n + 1);
            }}
          >
            重新读取
          </Button>
        </div>
      ) : !loading && items.length === 0 ? (
        <p>
          没有匹配的项目。<Link href="/projects">创建项目</Link>
        </p>
      ) : null}
    </div>
  );
}
