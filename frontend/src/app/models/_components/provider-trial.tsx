"use client";

import dynamic from "next/dynamic";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { createProviderTrialQuote, getConfigReceipt } from "@/api/settings";
import { listProjects } from "@/api/projects";
import { confirmOperationQuote, getOperation } from "@/api/operations";
import { useSession } from "@/components/auth/session";
import { catalogMessage } from "@/components/catalog/mutation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const TrialResult = dynamic(() =>
  import("@/components/generation/generation-dialog").then((m) => m.TaskDetail),
);

type PendingTrial = {
  request: string;
  confirm: string;
  project: string;
  operation?: string;
  confirmRequested?: boolean;
};
const event = "glowloom:trial-request";
function subscribe(callback: () => void) {
  window.addEventListener("storage", callback);
  window.addEventListener(event, callback);
  return () => {
    window.removeEventListener("storage", callback);
    window.removeEventListener(event, callback);
  };
}
function read(key: string) {
  try {
    return localStorage.getItem(key) ?? "";
  } catch {
    return "unavailable";
  }
}
function parse(raw: string): PendingTrial | null {
  try {
    const value = JSON.parse(raw);
    return typeof value.request === "string" &&
      typeof value.confirm === "string" &&
      typeof value.project === "string"
      ? value
      : null;
  } catch {
    return null;
  }
}
const terminal = new Set(["completed", "failed", "cancelled", "expired"]);

export function ProviderTrial({
  release,
  onVerified,
}: {
  release: API.ModelRelease;
  onVerified: () => void;
}) {
  const { actor } = useSession();
  const storageKey = `glowloom.trial.v1.${actor.id}.${release.id}`;
  const snapshot = useSyncExternalStore(
    subscribe,
    () => read(storageKey),
    () => "server",
  );
  const pending = parse(snapshot);
  const [projects, setProjects] = useState<API.ProjectResponse[]>([]);
  const [task, setTask] = useState<API.TaskDetail | null>(null);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  useEffect(() => {
    const abort = new AbortController();
    listProjects({ status: "active", limit: 200 }, { signal: abort.signal })
      .then((r) => {
        if (!abort.signal.aborted) setProjects(r.items ?? []);
      })
      .catch((e) => {
        if (!abort.signal.aborted) setMessage(catalogMessage(e));
      });
    return () => abort.abort();
  }, []);
  function save(value: PendingTrial) {
    localStorage.setItem(storageKey, JSON.stringify(value));
    window.dispatchEvent(new Event(event));
  }
  async function run(action: () => Promise<void>) {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setMessage("");
    try {
      await action();
    } catch (e) {
      setMessage(catalogMessage(e));
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  async function refresh(value: PendingTrial) {
    let operation = value.operation;
    if (!operation) {
      const receipt = await getConfigReceipt({
        id: value.request,
        action: "trial.quote",
        target_id: release.id!,
      });
      const result = receipt.result;
      if (
        typeof result?.operation_id !== "string" ||
        result.project_id !== value.project
      )
        throw new Error("试运行回执不匹配。");
      operation = result.operation_id;
      save({ ...value, operation });
    }
    const loaded = await getOperation({
      id: operation,
      project_id: value.project,
    });
    if (loaded.id !== operation || loaded.project_id !== value.project)
      throw new Error("试运行任务不匹配。");
    setTask(loaded);
    if (loaded.status === "completed") {
      setMessage("试运行已完成。刷新模型验证记录后即可检查上线条件。");
    }
  }
  const disabled = busy || snapshot === "server" || snapshot === "unavailable";
  return (
    <details className="space-y-3 rounded-lg border p-3">
      <summary className="cursor-pointer font-medium">试运行此发布</summary>
      <p className="text-sm text-muted-foreground">
        先准备报价，再确认收费执行。使用后台已保存的
        Key；每次试运行生成一个结果，费用记入所选项目。
      </p>
      {snapshot === "unavailable" && (
        <p role="alert">
          请允许浏览器保存请求标识，以便恢复未确认的操作。这里不会保存 Key
          或提示词。
        </p>
      )}
      {message && (
        <p role="status" className="text-sm">
          {message}
        </p>
      )}
      {!pending?.operation && (
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            const form = new FormData(e.currentTarget);
            void run(async () => {
              const project = String(form.get("project"));
              if (pending && pending.project !== project)
                throw new Error("请先恢复现有试运行，不能更换项目后重试。");
              const state = pending ?? {
                request: crypto.randomUUID(),
                confirm: crypto.randomUUID(),
                project,
              };
              const params = JSON.parse(String(form.get("params") || "{}"));
              const references = JSON.parse(
                String(form.get("references") || "[]"),
              );
              if (!Array.isArray(references))
                throw new Error("参考素材需要使用列表格式。");
              const charge = Math.round(Number(form.get("charge")) * 1_000_000);
              const supplier = Math.round(
                Number(form.get("supplier")) * 1_000_000,
              );
              if (
                !Number.isSafeInteger(charge) ||
                charge < 0 ||
                !Number.isSafeInteger(supplier) ||
                supplier < 0
              )
                throw new Error("预算必须是有效的非负金额。");
              save(state);
              const quote = await createProviderTrialQuote(
                { id: release.id! },
                {
                  project_id: project,
                  request_id: state.request,
                  mode: String(form.get("mode")),
                  prompt: String(form.get("prompt")),
                  params,
                  media_inputs: references,
                  max_charge_micros: charge,
                  supplier_max_micros: supplier,
                  supplier_currency: release.upstream_cost?.currency,
                },
              );
              if (!quote.operation_id)
                throw new Error("报价响应尚未确认，请查询回执。");
              const saved = { ...state, operation: quote.operation_id };
              save(saved);
              await refresh(saved);
            });
          }}
        >
          <label>
            扣费项目
            <select
              required
              name="project"
              defaultValue={pending?.project ?? ""}
              className="mt-1 w-full rounded-md border bg-background p-2"
            >
              <option value="" disabled>
                选择有权访问的项目
              </option>
              {projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            验证模式
            <select
              name="mode"
              required
              className="mt-1 w-full rounded-md border bg-background p-2"
            >
              {release.required_modes?.map((mode) => (
                <option key={mode}>{mode}</option>
              ))}
            </select>
          </label>
          <label>
            提示词
            <textarea
              name="prompt"
              required
              maxLength={65536}
              className="mt-1 min-h-24 w-full rounded-md border bg-background p-2"
            />
          </label>
          <div className="grid gap-3 sm:grid-cols-2">
            <label>
              本次平台费用上限（元）
              <Input
                name="charge"
                type="number"
                min="0"
                step="0.000001"
                required
              />
            </label>
            <label>
              供应商预算（{release.upstream_cost?.currency}）
              <Input
                name="supplier"
                type="number"
                min="0"
                step="0.000001"
                required
                defaultValue={
                  (release.upstream_cost?.max_micros ?? 0) / 1_000_000
                }
              />
            </label>
          </div>
          <details>
            <summary>模型参数与参考素材</summary>
            <label className="block">
              参数 JSON
              <textarea
                name="params"
                defaultValue="{}"
                className="mt-1 w-full rounded-md border bg-background p-2 font-mono"
              />
            </label>
            <label className="block">
              参考素材列表（role、media_asset_id）
              <textarea
                name="references"
                defaultValue="[]"
                className="mt-1 w-full rounded-md border bg-background p-2 font-mono"
              />
            </label>
          </details>
          <Button disabled={disabled}>准备免费报价</Button>
        </form>
      )}
      {pending && (
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          onClick={() => void run(() => refresh(pending))}
        >
          查询报价回执 / 刷新任务
        </Button>
      )}
      {task && (
        <div className="space-y-2 text-sm">
          <p>
            任务状态：{task.status} · 报价 ¥
            {((task.quote_micros ?? 0) / 1_000_000).toFixed(6)}
            {task.settled_micros !== undefined &&
              ` · 已结算 ¥${(task.settled_micros / 1_000_000).toFixed(6)}`}
          </p>
          {task.failure_code && <p role="alert">{task.failure_code}</p>}
          {task.status === "quoted" && pending && (
            <form
              className="space-y-2"
              onSubmit={(e) => {
                e.preventDefault();
                void run(async () => {
                  save({ ...pending, confirmRequested: true });
                  await confirmOperationQuote(
                    { id: task.id! },
                    { project_id: pending.project },
                    { headers: { "Idempotency-Key": pending.confirm } },
                  );
                  await refresh(pending);
                });
              }}
            >
              <label className="flex items-start gap-2">
                <input type="checkbox" required />
                我已确认提示词、模式及费用，开始真实供应商试运行。
              </label>
              <Button disabled={disabled}>确认费用并执行</Button>
            </form>
          )}
          {task.id && task.project_id && task.status !== "quoted" && (
            <details>
              <summary className="cursor-pointer underline">
                查看试运行结果与执行记录
              </summary>
              <TrialResult id={task.id} projectID={task.project_id} />
            </details>
          )}
          {task.status === "completed" && (
            <Button type="button" variant="outline" onClick={onVerified}>
              刷新模型验证记录
            </Button>
          )}
          {(terminal.has(task.status ?? "") ||
            (task.status === "quoted" && !pending?.confirmRequested)) && (
            <Button
              type="button"
              variant="outline"
              disabled={disabled}
              onClick={() => {
                localStorage.removeItem(storageKey);
                window.dispatchEvent(new Event(event));
                setTask(null);
                setMessage("");
              }}
            >
              准备另一项试运行
            </Button>
          )}
        </div>
      )}
    </details>
  );
}
