"use client";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { getWorkspace, requireScriptScope } from "./source-queries";
import type { ScriptScope } from "./source-intent";
import type { ScriptWorkspace } from "./source-model";
import { getFileImport, listFileImports } from "./file-import-queries";
import type { FileImportJob } from "./file-import-model";
import type { useFileImport } from "./use-file-import";
type Writer = ReturnType<typeof useFileImport>;
export function FileImportRecovery({ writer }: { writer: Writer }) {
  const recovery = useRef<HTMLButtonElement>(null);
  const [inspect, setInspect] = useState(false);
  const hasRecovery = Boolean(writer.intent || writer.storageError);
  useEffect(() => {
    if (!hasRecovery || writer.busy) return;
    const current = document.activeElement;
    if (current instanceof HTMLElement && current.matches(":disabled"))
      recovery.current?.focus();
  }, [hasRecovery, writer.busy, writer.intent?.key, writer.storageError]);
  if (!hasRecovery) return null;
  return (
    <section
      className="space-y-3 rounded-lg border border-amber-500 p-3"
      aria-label="未确认的文件导入请求"
    >
      <p>
        原请求尚未确认。核验前保留原件顺序、完整正文与原键，编辑和关闭保持锁定。
      </p>
      {writer.intent && (
        <>
          <p className="text-sm break-all">
            {writer.intent.action} · 原键 {writer.intent.key} · 原修订{" "}
            {writer.intent.body.expected_revision}
          </p>
          <details onToggle={(event) => setInspect(event.currentTarget.open)}>
            <summary className="cursor-pointer">查看完整原导入请求</summary>
            {inspect && (
              <pre className="max-h-64 overflow-auto text-xs break-all whitespace-pre-wrap">
                {JSON.stringify(writer.intent.body, null, 2)}
              </pre>
            )}
          </details>
        </>
      )}
      {writer.error && <p role="alert">{writer.error}</p>}
      {writer.storageError && <p role="alert">{writer.storageError}</p>}
      <Button
        ref={recovery}
        type="button"
        disabled={writer.busy}
        onClick={() =>
          void (writer.storageError ? writer.restoreStorage() : writer.replay())
        }
      >
        {writer.storageError
          ? "恢复导入存储与原意图"
          : "人工使用原键核验导入请求"}
      </Button>
    </section>
  );
}
export function FileImportConflict({
  scope,
  writer,
  onRebase,
  onDiscard,
}: {
  scope: ScriptScope;
  writer: Writer;
  onRebase?: (base: {
    expected_revision: number;
    base_version_id?: string;
  }) => void;
  onDiscard?: () => void;
}) {
  const [reading, setReading] = useState(false);
  const [facts, setFacts] = useState<{
    workspace: ScriptWorkspace;
    job?: FileImportJob;
  }>();
  const [error, setError] = useState<string>();
  const rejected = writer.rejected;
  if (!rejected)
    return writer.error && !writer.intent && !writer.storageError ? (
      <p role="alert">{writer.error}</p>
    ) : null;
  async function readLatest() {
    if (!rejected || reading) return;
    setReading(true);
    setError(undefined);
    setFacts(undefined);
    try {
      const [head, page, job] = await Promise.all([
        getWorkspace(scope.projectId),
        listFileImports(scope),
        rejected.intent.action === "create"
          ? Promise.resolve(undefined)
          : getFileImport(scope, rejected.intent.jobId),
      ]);
      requireScriptScope(head, scope);
      if (
        page.current_actor_id !== scope.actorId ||
        page.current_org_id !== scope.orgId ||
        window.location.origin !== scope.origin
      )
        throw new Error("当前主体或组织已变化，请保留原意图。");
      setFacts({ workspace: head, job });
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "最新导入事实读取未完成。",
      );
    } finally {
      setReading(false);
    }
  }
  const original = rejected.intent;
  return (
    <section
      className="space-y-3 rounded-lg border p-3"
      aria-label="被拒绝的导入草稿"
    >
      <p>
        原请求被确定拒绝，完整草稿保留。请读取当前事实并审阅后另明确新意图。
      </p>
      {writer.error && <p role="alert">{writer.error}</p>}
      <details>
        <summary className="cursor-pointer">保留的原导入草稿</summary>
        <pre className="max-h-52 overflow-auto text-xs break-all whitespace-pre-wrap">
          {JSON.stringify(original.body, null, 2)}
        </pre>
      </details>
      <Button
        type="button"
        variant="outline"
        disabled={reading}
        onClick={() => void readLatest()}
      >
        读取最新导入事实并保留草稿
      </Button>
      {error && <p role="alert">{error}</p>}
      {facts && (
        <>
          <p>
            当前脚本修订 {facts.workspace.state.revision}
            {facts.job
              ? `；当前任务修订 ${facts.job.revision}，状态 ${facts.job.status}`
              : ""}
            。
          </p>
          {original.action === "create" ? (
            <Button
              type="button"
              onClick={() => {
                const base = {
                  expected_revision: facts.workspace.state.revision,
                  ...(facts.workspace.state.draft_version_id
                    ? {
                        base_version_id: facts.workspace.state.draft_version_id,
                      }
                    : {}),
                };
                writer.acknowledgeLatest();
                if (onRebase) onRebase(base);
                else
                  void writer.submit({
                    action: "create",
                    body: {
                      ...original.body,
                      ...base,
                      base_version_id: facts.workspace.state.draft_version_id,
                    },
                  });
              }}
            >
              {onRebase
                ? "保留原件顺序，采用当前脚本版本"
                : "按此原件顺序另提交新导入意图"}
            </Button>
          ) : (
            <Button type="button" onClick={writer.acknowledgeLatest}>
              确认已读取当前任务，再选择新控制
            </Button>
          )}
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              writer.acknowledgeLatest();
              onDiscard?.();
            }}
          >
            明确放弃被拒绝的导入草稿
          </Button>
        </>
      )}
    </section>
  );
}
