"use client";

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Spinner } from "@/components/ui/spinner";
import { DownloadButton } from "@/components/media/download-button";
import { useSession } from "@/components/auth/session";
import {
  startAiTrial,
  getAiTrial,
  continueAiTrial,
  cancelAiTrial,
  recoverAiTrial,
  getAiTrialArtifact,
} from "@/api/settings";
import { RequestError } from "@/lib/request";
import { catalogMessage } from "@/components/catalog/mutation";

const trialMessages: Record<string, string> = {
  codex_unavailable: "无法使用本机 Codex，请检查 CLI 版本与登录状态。",
  codex_turn_unknown: "聊天结果尚未确认，请查询原会话。",
  trial_runtime_unavailable: "试验服务未就绪，请检查 Temporal 和宿主 worker。",
  trial_start_unconfirmed: "提交结果尚未确认，请查询原试验。",
  trial_turn_unconfirmed: "本轮提交尚未确认，请查询原会话。",
  trial_dispatch_unknown: "执行结果尚未确认，请查询原试验。",
  trial_cancel_unconfirmed: "已停止等待，远端状态尚未确认。",
  trial_result_unavailable: "尚未找到可读取的结果，请稍后查证。",
  trial_transfer_unconfirmed: "结果保存尚未确认，请查证原结果。",
  trial_query_timeout: "等待已结束，可继续查询原任务。",
  trial_not_found: "试验记录不存在或已过保存期，请新建试验。",
  trial_state_unavailable: "暂时无法读取试验状态，请稍后查询。",
  trial_unavailable: "当前无法执行试验，请检查平台连接及权限。",
  trial_request_conflict: "该请求已存在且内容不同，请查询原试验。",
  trial_session_unavailable: "会话不可继续，请查询状态或新建试验。",
  auth_failed: "供应商验证失败，请检查平台 Key。",
  insufficient_balance: "供应商余额不足，请检查账号。",
  invalid_parameters: "模型参数不被接受，请核对实际模型与参数。",
  rate_limited: "供应商请求过多，请稍后查询。",
};
function trialMessage(error: unknown) {
  return error instanceof RequestError
    ? (trialMessages[error.code] ?? catalogMessage(error))
    : catalogMessage(error);
}
function readTrialID(key: string) {
  try {
    const saved = localStorage.getItem(key);
    return saved && /^[0-9a-f-]{36}$/i.test(saved) ? saved : "";
  } catch {
    return "";
  }
}
function rememberTrial(key: string, id?: string) {
  try {
    if (id) localStorage.setItem(key, id);
    else localStorage.removeItem(key);
  } catch {
    /* Storage may be unavailable; the running trial still remains queryable. */
  }
}
const labels: Record<string, string> = {
  running: "执行中",
  pending: "生成中",
  succeeded: "已完成",
  unknown: "结果待确认",
  failed: "未完成",
  closed: "会话已结束",
};
export function ServiceTrial({
  open,
  onOpenChange,
  providers,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providers: API.ProviderListItem[];
}) {
  const { actor } = useSession();
  const storageKey = `glowloom.ai-trial.v1.${actor.id}`;
  const [kind, setKind] = useState("chat");
  const [source, setSource] = useState("codex");
  const [model, setModel] = useState("");
  const [provider, setProvider] = useState("");
  const [prompt, setPrompt] = useState("");
  const [reference, setReference] = useState("");
  const [size, setSize] = useState("2K");
  const [job, setJob] = useState(() => readTrialID(storageKey));
  const [restoredTurn] = useState(() => readTrialID(`${storageKey}.turn`));
  const [state, setState] = useState<API.TrialState>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [awaitingTurn, setAwaitingTurn] = useState(!!restoredTurn);
  const [models, setModels] = useState<string[]>([]);
  const [authenticated, setAuthenticated] = useState(false);
  const [imageSupported, setImageSupported] = useState(false);
  const [revision, setRevision] = useState(0);
  const expectedTurn = useRef(restoredTurn);
  const submitting = useRef(false);

  const choices = providers.filter((p) =>
    (kind === "video"
      ? ["volcengine_ark"]
      : ["volcengine_ark", "byteplus"]
    ).includes(p.provider?.adapter_key ?? ""),
  );
  useEffect(() => {
    if (!open || !job) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    let queries = 0;
    async function poll() {
      try {
        const result = await getAiTrial(
          { id: job },
          { signal: controller.signal },
        );
        if (controller.signal.aborted) return;
        setState(result);
        setError("");
        const last = result.turns?.at(-1);
        if (result.kind === "probe" && last?.state === "succeeded") {
          setModels(last.models ?? []);
          setAuthenticated(!!last.authenticated);
          setImageSupported(!!last.image_generation);
          setModel(last.models?.[0] ?? "");
        }
        if (
          result.state === "running" ||
          result.state === "pending" ||
          (result.state === "succeeded" &&
            expectedTurn.current &&
            last?.request_id !== expectedTurn.current &&
            ++queries < 50)
        )
          timer = setTimeout(poll, 1200);
        else {
          if (
            expectedTurn.current &&
            last?.request_id !== expectedTurn.current &&
            result.state === "succeeded"
          ) {
            setError("本轮提交尚未确认，请继续查询原会话。");
          } else {
            expectedTurn.current = "";
            setAwaitingTurn(false);
            rememberTrial(`${storageKey}.turn`);
          }
          setBusy(false);
        }
      } catch (e) {
        if (!controller.signal.aborted) {
          setError(trialMessage(e));
          setBusy(false);
        }
      }
    }
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [open, job, revision, storageKey]);
  function reset() {
    setJob("");
    setState(undefined);
    setError("");
    setPrompt("");
    expectedTurn.current = "";
    setAwaitingTurn(false);
    rememberTrial(storageKey);
    rememberTrial(`${storageKey}.turn`);
  }
  async function start(probe = false) {
    if (submitting.current) return;
    submitting.current = true;
    setBusy(true);
    setError("");
    const id = crypto.randomUUID();
    setState(undefined);
    expectedTurn.current = id;
    setAwaitingTurn(true);
    rememberTrial(storageKey, id);
    try {
      await startAiTrial({
        request_id: id,
        source: probe ? "codex" : source,
        kind: probe ? "probe" : kind,
        model: probe ? "" : model,
        prompt: probe ? "" : prompt,
        provider_id:
          probe || source === "codex"
            ? "00000000-0000-0000-0000-000000000000"
            : provider,
        mode:
          !probe && source === "cloud"
            ? kind === "video"
              ? reference
                ? "image2video"
                : "text2video"
              : reference
                ? "image2image"
                : "text2image"
            : "",
        params:
          probe || source === "codex"
            ? {}
            : kind === "image"
              ? { size }
              : { duration: 5, resolution: "720p" },
        references:
          probe || source === "codex" || !reference
            ? []
            : [
                {
                  role: kind === "video" ? "first_frame" : "reference_image",
                  url: reference,
                },
              ],
      });
      setJob(id);
      if (!probe) setPrompt("");
      setRevision((v) => v + 1);
    } catch (e) {
      setJob(id);
      setError(trialMessage(e));
      setBusy(false);
    } finally {
      submitting.current = false;
    }
  }
  async function send() {
    if (submitting.current) return;
    submitting.current = true;
    setBusy(true);
    setError("");
    const id = crypto.randomUUID();
    expectedTurn.current = id;
    setAwaitingTurn(true);
    rememberTrial(`${storageKey}.turn`, id);
    try {
      await continueAiTrial({ id: job }, { request_id: id, prompt });
      setPrompt("");
      setRevision((v) => v + 1);
    } catch (e) {
      setError(trialMessage(e));
      if (e instanceof RequestError && [401, 403, 422].includes(e.status)) {
        expectedTurn.current = "";
        setAwaitingTurn(false);
        rememberTrial(`${storageKey}.turn`);
      }
      setRevision((v) => v + 1);
    } finally {
      submitting.current = false;
    }
  }
  const last = state?.turns?.at(-1);
  const chat = state?.kind === "chat";
  const inFlight =
    busy || state?.state === "running" || state?.state === "pending";
  const conversationReady =
    chat &&
    !awaitingTurn &&
    state?.state === "succeeded" &&
    (state.turns?.length ?? 0) < 32;
  const artifact = last?.artifacts?.[0];
  const url = job
    ? `/api/admin/ai-trials/${encodeURIComponent(job)}/artifacts/1`
    : "";
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>开发试验</DialogTitle>
          <DialogDescription className="sr-only">
            验证本机 Codex、Seedream 和视频生成。
          </DialogDescription>
        </DialogHeader>
        <div className="flex min-h-0 flex-col gap-5 overflow-y-auto">
          {!job || state?.kind === "probe" ? (
            <>
              <div className="grid grid-cols-2 gap-3">
                <Field>
                  <FieldLabel htmlFor="trial-kind">能力</FieldLabel>
                  <Select
                    value={kind}
                    onValueChange={(v) => {
                      setKind(v);
                      if (v === "chat") {
                        setSource("codex");
                        setModel(models[0] ?? "");
                      }
                      if (v === "video") {
                        setSource("cloud");
                        setModel("");
                      }
                    }}
                    disabled={inFlight}
                  >
                    <SelectTrigger id="trial-kind" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="chat">聊天</SelectItem>
                      <SelectItem value="image">生图</SelectItem>
                      <SelectItem value="video">视频</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
                <Field>
                  <FieldLabel htmlFor="trial-source">服务</FieldLabel>
                  <Select
                    value={source}
                    onValueChange={(v) => {
                      setSource(v);
                      setModel(v === "codex" ? (models[0] ?? "") : "");
                    }}
                    disabled={inFlight}
                  >
                    <SelectTrigger id="trial-source" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {kind !== "video" ? (
                        <SelectItem value="codex">本机 Codex</SelectItem>
                      ) : null}
                      {kind !== "chat" ? (
                        <SelectItem value="cloud">云端模型</SelectItem>
                      ) : null}
                    </SelectContent>
                  </Select>
                </Field>
              </div>
              {source === "codex" ? (
                <div className="flex flex-wrap items-center gap-3">
                  <Button
                    variant="outline"
                    disabled={inFlight}
                    onClick={() => void start(true)}
                  >
                    检查 Codex
                  </Button>
                  {models.length ? (
                    <Badge variant="secondary">
                      {authenticated ? "已登录" : "未登录"}
                      {imageSupported ? " · 支持生图" : ""}
                    </Badge>
                  ) : null}
                </div>
              ) : (
                <Field>
                  <FieldLabel htmlFor="trial-provider">平台连接</FieldLabel>
                  <Select
                    value={provider}
                    onValueChange={setProvider}
                    disabled={inFlight}
                  >
                    <SelectTrigger id="trial-provider" className="w-full">
                      <SelectValue placeholder="选择连接" />
                    </SelectTrigger>
                    <SelectContent>
                      {choices.map((p) => (
                        <SelectItem
                          key={p.provider!.id}
                          value={p.provider!.id!}
                        >
                          {p.provider!.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
              )}
              <Field>
                <FieldLabel htmlFor="trial-model">模型 ID</FieldLabel>
                {source === "codex" && models.length ? (
                  <Select
                    value={model}
                    onValueChange={setModel}
                    disabled={inFlight}
                  >
                    <SelectTrigger id="trial-model" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {models.map((id) => (
                        <SelectItem key={id} value={id}>
                          {id}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                ) : (
                  <Input
                    id="trial-model"
                    value={model}
                    onChange={(e) => setModel(e.target.value)}
                    placeholder="填写账号可用的模型 ID"
                    disabled={inFlight}
                  />
                )}
              </Field>
              {source === "cloud" ? (
                <>
                  <Field>
                    <FieldLabel htmlFor="trial-reference">
                      参考图 URL（可选）
                    </FieldLabel>
                    <Input
                      id="trial-reference"
                      type="url"
                      value={reference}
                      onChange={(e) => setReference(e.target.value)}
                      placeholder="https://"
                      disabled={inFlight}
                    />
                  </Field>
                  {kind === "image" ? (
                    <Field>
                      <FieldLabel htmlFor="trial-size">尺寸</FieldLabel>
                      <Input
                        id="trial-size"
                        value={size}
                        onChange={(e) => setSize(e.target.value)}
                        disabled={inFlight}
                      />
                    </Field>
                  ) : null}
                </>
              ) : null}
            </>
          ) : (
            <div className="flex items-center justify-between gap-3">
              <span className="min-w-0 truncate text-sm">
                {state?.model ?? model}
              </span>
              <Badge variant="secondary">
                {labels[state?.state ?? ""] ?? "查询中"}
              </Badge>
            </div>
          )}
          {state?.turns?.map((turn, i) => (
            <div
              key={turn.request_id ?? i}
              className="flex flex-col gap-3 text-sm"
            >
              {turn.prompt ? (
                <p className="ml-8 rounded-lg bg-muted p-3 whitespace-pre-wrap">
                  {turn.prompt}
                </p>
              ) : null}
              {turn.text ? (
                <p className="mr-4 whitespace-pre-wrap">{turn.text}</p>
              ) : null}
              {turn.error_code ? (
                <p role="status" className="text-muted-foreground">
                  {labels[turn.state ?? ""] ?? "未完成"} ·{" "}
                  {trialMessages[turn.error_code] ??
                    "服务未完成本次试验，请检查连接或查询状态。"}
                </p>
              ) : null}
            </div>
          ))}
          {artifact && state?.state === "succeeded" ? (
            <div className="flex flex-col gap-3">
              {artifact.mime?.startsWith("image/") ? (
                <Image
                  src={url}
                  alt="生成结果"
                  width={768}
                  height={768}
                  unoptimized
                  className="h-auto w-full rounded-lg"
                />
              ) : (
                <video src={url} controls className="w-full rounded-lg" />
              )}
              <DownloadButton
                filename={
                  artifact.mime?.startsWith("video/")
                    ? "result.mp4"
                    : "result.png"
                }
                load={() =>
                  getAiTrialArtifact(
                    { id: job, sequence: 1 },
                    { responseType: "blob" },
                  )
                }
              >
                下载结果
              </DownloadButton>
            </div>
          ) : null}
          {state?.kind !== "probe" && state?.state === "unknown" ? (
            <p className="text-sm text-muted-foreground">
              结果尚未确认，查询不会再次发起生成。
            </p>
          ) : null}
          {!job || state?.kind === "probe" || conversationReady ? (
            <Field>
              <FieldLabel htmlFor="trial-prompt">
                {conversationReady ? "继续聊天" : "提示词"}
              </FieldLabel>
              <Textarea
                id="trial-prompt"
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                maxLength={32768}
                disabled={inFlight}
              />
            </Field>
          ) : null}
          {error ? (
            <p role="alert" className="text-sm">
              {error}
            </p>
          ) : null}
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-2 border-t pt-4">
          {job ? (
            <Button variant="ghost" disabled={inFlight} onClick={reset}>
              新试验
            </Button>
          ) : null}
          {job ? (
            <Button variant="outline" onClick={() => setRevision((v) => v + 1)}>
              查询状态
            </Button>
          ) : null}
          {state?.state === "unknown" &&
          (state.kind === "image" || state.kind === "video") ? (
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => {
                setBusy(true);
                void recoverAiTrial({ id: job })
                  .then(() => setRevision((v) => v + 1))
                  .catch((e) => {
                    setError(trialMessage(e));
                    setBusy(false);
                  });
              }}
            >
              查证结果
            </Button>
          ) : null}
          {inFlight && job ? (
            <Button
              variant="outline"
              onClick={() =>
                void cancelAiTrial({ id: job })
                  .then(() => setRevision((v) => v + 1))
                  .catch((e) => setError(trialMessage(e)))
              }
            >
              停止
            </Button>
          ) : null}
          {!job || state?.kind === "probe" || conversationReady ? (
            <Button
              disabled={
                inFlight ||
                !prompt.trim() ||
                !model ||
                (source === "cloud" && !provider) ||
                (!conversationReady &&
                  source === "codex" &&
                  (!authenticated || (kind === "image" && !imageSupported)))
              }
              onClick={() => void (conversationReady ? send() : start())}
            >
              {inFlight ? <Spinner /> : null}
              {conversationReady ? "发送" : "开始试验"}
            </Button>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  );
}
