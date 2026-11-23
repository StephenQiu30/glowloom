"use client";

import { useId, useRef, useState } from "react";
import useSWR from "swr";
import {
  listProjectMemory,
  listProjectMemoryHistory,
  publishProjectMemory,
} from "@/api/projectMemory";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { readErrorMessage } from "@/lib/read-error";
import { RequestError } from "@/lib/request";

type Draft = { key: string; statement: string; revision: number };

export function ProjectMemoryDialog({
  projectID,
  initialStatement,
  shot,
  onClose,
}: {
  projectID: string;
  initialStatement: string;
  shot?: { id: string; title: string };
  onClose: () => void;
}) {
  const fieldID = useId();
  const shotID = shot?.id;
  const [draft, setDraft] = useState<Draft>({
    key: "",
    statement: initialStatement,
    revision: 0,
  });
  const [historyKey, setHistoryKey] = useState("");
  const [feedback, setFeedback] = useState("");
  const [saving, setSaving] = useState(false);
  const busy = useRef(false);
  const attempt = useRef<{ input: string; id: string } | null>(null);
  const { data, error, mutate } = useSWR(
    ["project-memory", projectID, shotID],
    ([, pid, shot_id]) => listProjectMemory({ pid, shot_id }),
  );
  const history = useSWR(
    historyKey
      ? ["project-memory-history", projectID, historyKey, shotID]
      : null,
    ([, pid, key, shot_id]) =>
      listProjectMemoryHistory({ pid, key: encodeURIComponent(key), shot_id }),
  );

  const key = draft.key.trim();
  const statement = draft.statement.trim();
  const validKey =
    /^[\p{L}\p{N}_-]+$/u.test(key) && Array.from(key).length <= 48;
  const canSubmit =
    !!data?.can_edit &&
    !error &&
    validKey &&
    !!statement &&
    Array.from(statement).length <= 512;
  const items = data?.items ?? [];

  async function publish(withdraw: boolean) {
    if (
      busy.current ||
      !data?.can_edit ||
      error ||
      (withdraw ? draft.revision < 1 : !canSubmit)
    )
      return;
    const input = {
      expected_revision: draft.revision,
      ...(shotID ? { shot_id: shotID } : {}),
      ...(withdraw ? { withdraw: true } : { statement }),
    };
    const identity = JSON.stringify([key, input]);
    if (attempt.current?.input !== identity)
      attempt.current = { input: identity, id: crypto.randomUUID() };
    busy.current = true;
    setSaving(true);
    setFeedback("");
    try {
      const result = await publishProjectMemory(
        { pid: projectID, key: encodeURIComponent(key) },
        input,
        {
          headers: {
            "Content-Type": "application/json",
            "Idempotency-Key": attempt.current.id,
          },
        },
      );
      if (!result.revision) {
        setFeedback("确认结果暂不可读，请重试同一次操作。");
        return;
      }
      attempt.current = null;
      setFeedback(
        withdraw
          ? "已撤回约定。"
          : shotID
            ? "已发布镜头约定。"
            : "已发布。下一次新会话将读取这条约定。",
      );
      setDraft({ key: "", statement: "", revision: 0 });
      setHistoryKey(key);
      void mutate();
      if (historyKey === key) void history.mutate();
    } catch (cause) {
      if (cause instanceof RequestError && cause.code === "revision_conflict") {
        void mutate();
        setHistoryKey(key);
        setFeedback("约定已变更。请查看最新修订，再选择要更正的版本。");
      } else if (
        cause instanceof RequestError &&
        cause.code === "project_memory_limit"
      ) {
        setFeedback(
          shotID
            ? "此镜头已有 12 条有效约定，请先撤回一条。"
            : "项目已有 24 条有效约定，请先撤回一条。",
        );
      } else if (
        cause instanceof RequestError &&
        cause.code === "state_conflict"
      ) {
        setFeedback("项目当前为只读状态。");
        void mutate();
      } else {
        setFeedback(readErrorMessage(cause));
      }
    } finally {
      busy.current = false;
      setSaving(false);
    }
  }

  function edit(item: API.ProjectMemory) {
    if (!item.key || !item.revision) return;
    setDraft({
      key: item.key,
      statement: item.statement ?? "",
      revision: item.revision,
    });
    setFeedback("");
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy.current) onClose();
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {shot ? `镜头约定 · ${shot.title}` : "项目记忆"}
          </DialogTitle>
          <DialogDescription>
            {shot
              ? "此约定与项目成员共享，只用于当前镜头。同名的项目约定以此镜头的值为准。"
              : "发布的内容会共享给有权查看此项目的成员，并带入他们的新会话。"}
          </DialogDescription>
        </DialogHeader>
        {error ? (
          <p role="alert">{readErrorMessage(error)}</p>
        ) : !data ? (
          <p role="status">正在读取约定…</p>
        ) : (
          <ul className="divide-y">
            {items.length === 0 && (
              <li className="py-2 text-muted-foreground">暂无已发布约定</li>
            )}
            {items.map((item) => (
              <li key={item.id} className="space-y-1 py-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="font-medium">{item.key}</span>
                  <div className="flex gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setHistoryKey(item.key ?? "")}
                    >
                      修订 {item.revision}
                    </Button>
                    {data.can_edit && (
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={saving}
                        onClick={() => edit(item)}
                      >
                        更正
                      </Button>
                    )}
                  </div>
                </div>
                <p className="break-words whitespace-pre-wrap">
                  {item.statement}
                </p>
              </li>
            ))}
          </ul>
        )}
        {historyKey && (
          <section
            aria-label="约定修订记录"
            className="space-y-2 rounded-lg border p-3"
          >
            <p className="font-medium">{historyKey} 的修订记录</p>
            {history.error && (
              <p role="alert">{readErrorMessage(history.error)}</p>
            )}
            {history.data?.length === 0 && <p>此名称暂无修订记录。</p>}
            {history.data?.map((item, index) => (
              <div key={item.id} className="space-y-1 break-words">
                <p>
                  第 {item.revision} 版{item.withdrawn ? " · 已撤回" : ""}
                </p>
                {item.statement && (
                  <p className="whitespace-pre-wrap text-muted-foreground">
                    {item.statement}
                  </p>
                )}
                {index === 0 && data?.can_edit && item.withdrawn && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={saving}
                    onClick={() => edit(item)}
                  >
                    重新发布
                  </Button>
                )}
              </div>
            ))}
          </section>
        )}
        {data?.can_edit && !error && (
          <form
            className="space-y-3"
            onSubmit={(event) => {
              event.preventDefault();
              void publish(false);
            }}
          >
            <div className="space-y-1">
              <Label htmlFor={fieldID + "-key"}>约定名称</Label>
              <Input
                id={fieldID + "-key"}
                value={draft.key}
                maxLength={48}
                placeholder="例如：声音"
                disabled={saving || draft.revision > 0}
                onChange={(event) =>
                  setDraft({ ...draft, key: event.target.value })
                }
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor={fieldID + "-statement"}>确认共享的内容</Label>
              <Textarea
                id={fieldID + "-statement"}
                value={draft.statement}
                placeholder={
                  shot ? "例如：此镜保留一句独白" : "例如：全片保留同期声"
                }
                disabled={saving}
                onChange={(event) =>
                  setDraft({ ...draft, statement: event.target.value })
                }
              />
            </div>
            {Array.from(statement).length > 512 && (
              <p role="status">请将约定精简到 512 字以内。</p>
            )}
            <div className="flex flex-wrap gap-2">
              {validKey && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={saving}
                  onClick={() => setHistoryKey(key)}
                >
                  查看修订
                </Button>
              )}
              <Button type="submit" disabled={!canSubmit || saving}>
                {draft.revision ? "确认更正" : "确认发布"}
              </Button>
              {draft.revision > 0 && items.some((item) => item.key === key) && (
                <Button
                  type="button"
                  variant="outline"
                  disabled={saving}
                  onClick={() => void publish(true)}
                >
                  撤回约定
                </Button>
              )}
              {draft.revision > 0 && (
                <Button
                  type="button"
                  variant="ghost"
                  disabled={saving}
                  onClick={() =>
                    setDraft({ key: "", statement: "", revision: 0 })
                  }
                >
                  添加新约定
                </Button>
              )}
            </div>
          </form>
        )}
        {feedback && <p role="status">{feedback}</p>}
      </DialogContent>
    </Dialog>
  );
}
