"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { useCallback, useEffect, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { Plus, Search, UserRound, MapPin, Box } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  InputGroup,
  InputGroupInput,
  InputGroupAddon,
} from "@/components/ui/input-group";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
  EmptyMedia,
  EmptyContent,
} from "@/components/ui/empty";
import { listBibleEntries, getBibleEntry } from "@/api/bible";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { RequestError } from "@/lib/request";
import { flowError } from "@/components/generation/contracts";
import { isProjectID } from "@/components/projects/project";
import {
  kinds,
  requireSubject,
  subjectImage,
  subjectText,
  type Subject,
} from "./contracts";
import { SubjectEditor } from "./subject-editor";
import { SubjectDetail } from "./subject-detail";
export function SubjectList({
  projectID,
  kind = "character",
  onSelect,
  selectedID,
  onGenerate,
  filters,
  searchQuery,
}: {
  projectID: string;
  kind?: API.Kind;
  onSelect?: (s: Subject) => void;
  selectedID?: string;
  onGenerate?: (s: Subject) => void;
  filters?: ReactNode;
  searchQuery?: string;
}) {
  const [items, setItems] = useState<Subject[]>([]),
    [cursor, setCursor] = useState<string | undefined>(),
    [localQuery, setQuery] = useState(""),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [retry, setRetry] = useState(0),
    [create, setCreate] = useState(false),
    [selected, setSelected] = useState<Subject | null>(null),
    [deleted, setDeleted] = useState(false);
  const query = searchQuery ?? localQuery;
  const read = useCallback(
    async (next?: string, signal?: AbortSignal) => {
      const p = await listBibleEntries(
        { pid: projectID, kind, deleted, limit: 24, cursor: next },
        { signal },
      );
      if (
        !Array.isArray(p.entries) ||
        p.entries.some(
          (s) =>
            !isProjectID(s.head?.id) ||
            s.head?.project_id !== projectID ||
            s.head.kind !== kind,
        )
      )
        throw new RequestError("invalid_receipt");
      const entries = await Promise.all(
        p.entries.map(async (s) =>
          requireSubject(
            await getBibleEntry(
              { pid: projectID, kind, id: s.head!.id! },
              { signal },
            ),
            projectID,
            kind,
            s.head!.id,
          ),
        ),
      );
      return { items: entries, cursor: p.next_cursor };
    },
    [projectID, kind, deleted],
  );
  useEffect(() => {
    const c = new AbortController();
    void read(undefined, c.signal)
      .then((p) => {
        if (!c.signal.aborted) {
          setItems(p.items);
          setCursor(p.cursor);
          setError("");
        }
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(flowError(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [read, retry]);
  async function more() {
    if (loading) return;
    setLoading(true);
    try {
      const p = await read(cursor);
      setItems((v) => [
        ...v,
        ...p.items.filter((s) => !v.some((old) => old.head.id === s.head.id)),
      ]);
      setCursor(p.cursor);
      setError("");
    } catch (e) {
      setError(flowError(e));
    } finally {
      setLoading(false);
    }
  }
  function change(s: Subject) {
    setItems((v) => [s, ...v.filter((i) => i.head.id !== s.head.id)]);
  }
  const shown = items.filter(
    (s) =>
      !!s.head.deleted === deleted &&
      subjectText(s).name?.toLowerCase().includes(query.trim().toLowerCase()),
  );
  const Icon =
    kind === "character" ? UserRound : kind === "location" ? MapPin : Box;
  return (
    <>
      <div className="subject-toolbar">
        {searchQuery === undefined ? (
          <InputGroup className="subject-search">
            <InputGroupAddon>
              <Search />
            </InputGroupAddon>
            <InputGroupInput
              aria-label="搜索主体"
              placeholder="搜索已加载主体"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              maxLength={100}
            />
          </InputGroup>
        ) : null}
        {filters}
        <Button
          variant={deleted ? "outline" : "secondary"}
          disabled={!deleted}
          onClick={() => {
            setLoading(true);
            setItems([]);
            setCursor(undefined);
            setDeleted(false);
          }}
        >
          可用主体
        </Button>
        {!onSelect && (
          <Button
            variant={deleted ? "secondary" : "outline"}
            disabled={deleted}
            onClick={() => {
              setLoading(true);
              setItems([]);
              setCursor(undefined);
              setDeleted(true);
            }}
          >
            已删除
          </Button>
        )}
        <Button className="ml-auto" onClick={() => setCreate(true)}>
          <Plus />
          创建{kinds[kind]}
        </Button>
      </div>
      {error && (
        <FeedbackToast
          message={error}
          action={{
            label: "重试读取",
            onClick: () => {
              setError("");
              setLoading(true);
              setRetry((n) => n + 1);
            },
          }}
        />
      )}
      {loading && items.length === 0 ? (
        <div className="subject-grid" aria-label="正在读取主体">
          {[0, 1, 2, 3].map((n) => (
            <Skeleton className="subject-skeleton" key={n} />
          ))}
        </div>
      ) : shown.length === 0 && !error ? (
        <Empty className="min-h-80">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Icon />
            </EmptyMedia>
            <EmptyTitle>
              {query
                ? "没有匹配的主体"
                : deleted
                  ? "没有已删除的主体"
                  : "还没有" + kinds[kind] + "主体"}
            </EmptyTitle>
            <EmptyDescription>
              {query
                ? "换个关键词，或加载更多主体。"
                : "添加参考与描述，在创作中保持一致。"}
            </EmptyDescription>
          </EmptyHeader>
          {!query && !deleted && (
            <EmptyContent>
              <Button variant="outline" onClick={() => setCreate(true)}>
                创建第一个{kinds[kind]}
              </Button>
            </EmptyContent>
          )}
        </Empty>
      ) : (
        <div className="subject-grid">
          {!deleted && !query && (
            <Button
              variant="tile"
              className="subject-create"
              onClick={() => setCreate(true)}
            >
              <Plus />
              <span>新建主体</span>
              <small>添加主图、名称与描述</small>
            </Button>
          )}
          {shown.map((s) => {
            const image = subjectImage(s);
            return (
              <Button
                variant="ghost"
                className="subject-card h-auto p-0 whitespace-normal"
                key={s.head.id}
                aria-pressed={onSelect ? selectedID === s.head.id : undefined}
                onClick={() => (onSelect ? onSelect(s) : setSelected(s))}
              >
                <Card variant="media" className="w-full">
                  <CardContent>
                    <div className="subject-card-image">
                      {image ? (
                        <ProjectMediaPreview projectID={projectID} id={image} />
                      ) : (
                        <Icon size={48} strokeWidth={1} />
                      )}
                      <Badge variant="secondary" className="subject-status">
                        {s.head.deleted
                          ? "已删除"
                          : s.head.confirmed_version_id === s.current.id
                            ? "已确认"
                            : "草稿"}
                      </Badge>
                    </div>
                  </CardContent>
                  <CardHeader>
                    <CardTitle>{subjectText(s).name}</CardTitle>
                    <CardDescription className="truncate">
                      {subjectText(s).description || "添加描述，让创作更一致"}
                    </CardDescription>
                  </CardHeader>
                </Card>
              </Button>
            );
          })}
        </div>
      )}
      {cursor && (
        <Button
          className="mx-auto mt-5 flex"
          variant="outline"
          disabled={loading}
          onClick={() => void more()}
        >
          {loading ? "正在读取…" : "加载更多主体"}
        </Button>
      )}
      {deleted && (
        <p className="flow-notice">
          已删除主体仍保留版本记录，恢复后可继续编辑与引用。
        </p>
      )}
      {create && (
        <SubjectEditor
          projectID={projectID}
          kind={kind}
          onClose={() => setCreate(false)}
          onSaved={(s) => {
            change(s);
            setCreate(false);
            setSelected(s);
          }}
        />
      )}
      {selected && (
        <SubjectDetail
          subject={selected}
          onClose={() => setSelected(null)}
          onChanged={(s) => {
            change(s);
            if (!!s.head.deleted !== deleted) {
              setSelected(null);
              toast.success(
                s.head.deleted ? "主体已移除" : "主体已恢复",
                s.head.deleted
                  ? {
                      description: "历史版本保留，可在已删除列表中恢复。",
                      action: {
                        label: "查看已删除",
                        onClick: () => {
                          setDeleted(true);
                          setLoading(true);
                          setItems([]);
                          setCursor(undefined);
                        },
                      },
                    }
                  : undefined,
              );
              if (!s.head.deleted) {
                setDeleted(false);
                setLoading(true);
                setItems([]);
                setCursor(undefined);
              }
            }
          }}
          onUse={
            onSelect ??
            (onGenerate
              ? (s) => {
                  setSelected(null);
                  onGenerate(s);
                }
              : undefined)
          }
        />
      )}
    </>
  );
}
