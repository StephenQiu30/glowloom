"use client";
import { pageBusinessMemory } from "@/lib/page-business-memory";

import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { ChevronDown } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { Checkbox } from "@/components/ui/checkbox";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

import dynamic from "next/dynamic";
import {
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
  useId,
} from "react";
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
    return pageBusinessMemory.getItem(key) ?? "";
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
  const formFieldId = useId();

  const { actor } = useSession();
  const storageKey = `glowloom.trial.v1.${actor.id}.${release.id}`;
  const snapshot = useSyncExternalStore(
    subscribe,
    () => read(storageKey),
    () => "server",
  );
  const pending = parse(snapshot);
  const [selectedMode, setSelectedMode] = useState(
    release.required_modes?.[0] ?? "",
  );
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
    pageBusinessMemory.setItem(storageKey, JSON.stringify(value));
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
    <Collapsible className="space-y-3 rounded-lg border p-3">
      <CollapsibleTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          className="group/collapsible h-auto w-full justify-between whitespace-normal"
        >
          <span>试运行此发布</span>
          <ChevronDown
            data-icon="inline-end"
            className="transition-transform group-data-[state=open]/collapsible:rotate-180"
          />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent
        forceMount
        className="pt-3 data-[state=closed]:hidden"
      >
        <p className="text-sm text-muted-foreground">
          先准备报价，再确认收费执行。使用后台已保存的
          Key；每次试运行生成一个结果，费用记入所选项目。
        </p>
        {snapshot === "unavailable" && (
          <Alert variant="destructive">
            <AlertDescription>
              请允许浏览器保存请求标识，以便恢复未确认的操作。这里不会保存 Key
              或提示词。
            </AlertDescription>
          </Alert>
        )}
        {message && <FeedbackToast message={message} tone="info" />}
        {!pending?.operation && (
          <form
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
                const charge = Math.round(
                  Number(form.get("charge")) * 1_000_000,
                );
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
                    prompt: String(form.get("prompt") ?? ""),
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
            <FieldGroup className="grid gap-3">
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-1`}>
                  <span>扣费项目</span>
                </FieldLabel>
                <Select
                  required
                  name="project"
                  defaultValue={pending?.project ?? ""}
                >
                  <SelectTrigger
                    id={`${formFieldId}-field-1`}
                    className="mt-1 w-full"
                  >
                    <SelectValue placeholder={"选择有权访问的项目"} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {projects.map((p) =>
                        p.id ? (
                          <SelectItem key={p.id} value={p.id}>
                            {p.name}
                          </SelectItem>
                        ) : null,
                      )}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-2`}>
                  <span>验证模式</span>
                </FieldLabel>
                <Select
                  name="mode"
                  required
                  value={selectedMode}
                  onValueChange={setSelectedMode}
                >
                  <SelectTrigger
                    id={`${formFieldId}-field-2`}
                    className="mt-1 w-full"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {release.required_modes?.map((mode) => (
                        <SelectItem key={mode} value={mode}>
                          {mode}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-3`}>
                  <span>提示词</span>
                </FieldLabel>
                <Textarea
                  id={`${formFieldId}-field-3`}
                  name="prompt"
                  required={!["avatar", "motion"].includes(selectedMode)}
                  disabled={selectedMode === "motion"}
                  maxLength={65536}
                  className="mt-1 min-h-24 w-full"
                />
              </Field>
              <div className="grid gap-3 sm:grid-cols-2">
                <Field>
                  <FieldLabel htmlFor={`${formFieldId}-field-4`}>
                    <span>本次平台费用上限（元）</span>
                  </FieldLabel>
                  <Input
                    id={`${formFieldId}-field-4`}
                    name="charge"
                    type="number"
                    min="0"
                    step="0.000001"
                    required
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${formFieldId}-field-5`}>
                    <span>供应商预算（{release.upstream_cost?.currency}）</span>
                  </FieldLabel>
                  <Input
                    id={`${formFieldId}-field-5`}
                    name="supplier"
                    type="number"
                    min="0"
                    step="0.000001"
                    required
                    defaultValue={
                      (release.upstream_cost?.max_micros ?? 0) / 1_000_000
                    }
                  />
                </Field>
              </div>
              <Collapsible>
                <CollapsibleTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    className="group/collapsible h-auto w-full justify-between whitespace-normal"
                  >
                    <span>模型参数与参考素材</span>
                    <ChevronDown
                      data-icon="inline-end"
                      className="transition-transform group-data-[state=open]/collapsible:rotate-180"
                    />
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent
                  forceMount
                  className="pt-3 data-[state=closed]:hidden"
                >
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-6`}>
                      <span>参数 JSON</span>
                    </FieldLabel>
                    <Textarea
                      id={`${formFieldId}-field-6`}
                      name="params"
                      defaultValue="{}"
                      className="mt-1 w-full"
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-7`}>
                      <span>参考素材列表（role、media_asset_id）</span>
                    </FieldLabel>
                    <Textarea
                      id={`${formFieldId}-field-7`}
                      name="references"
                      defaultValue="[]"
                      className="mt-1 w-full"
                    />
                  </Field>
                </CollapsibleContent>
              </Collapsible>
              <Button disabled={disabled}>准备免费报价</Button>
            </FieldGroup>
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
            {task.failure_code && (
              <Alert variant="destructive">
                <AlertDescription>{task.failure_code}</AlertDescription>
              </Alert>
            )}
            {task.status === "quoted" && pending && (
              <form
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
                <FieldGroup className="gap-2">
                  <Field orientation="horizontal">
                    <Checkbox id={`${formFieldId}-field-8`} required />
                    <FieldLabel htmlFor={`${formFieldId}-field-8`}>
                      <span>
                        我已确认提示词、模式及费用，开始真实供应商试运行。
                      </span>
                    </FieldLabel>
                  </Field>
                  <Button disabled={disabled}>确认费用并执行</Button>
                </FieldGroup>
              </form>
            )}
            {task.id && task.project_id && task.status !== "quoted" && (
              <Collapsible>
                <CollapsibleTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    className="group/collapsible h-auto w-full justify-between whitespace-normal"
                  >
                    <span>查看试运行结果与执行记录</span>
                    <ChevronDown
                      data-icon="inline-end"
                      className="transition-transform group-data-[state=open]/collapsible:rotate-180"
                    />
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent
                  forceMount
                  className="pt-3 data-[state=closed]:hidden"
                >
                  <TrialResult id={task.id} projectID={task.project_id} />
                </CollapsibleContent>
              </Collapsible>
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
                  pageBusinessMemory.removeItem(storageKey);
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
      </CollapsibleContent>
    </Collapsible>
  );
}
