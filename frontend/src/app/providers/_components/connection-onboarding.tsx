"use client";

import { useId, useRef, useState, type ReactNode } from "react";
import useSWR from "swr";
import copy from "copy-to-clipboard";
import { toast } from "sonner";
import { providerConsoleURL } from "./provider-label";
import { useSession } from "@/components/auth/session";
import { Check } from "lucide-react";
import {
  getProviderOnboarding,
  testProviderOnboardingModel,
  queryProviderModelCheck,
  endProviderCheckTracking,
} from "@/api/settings";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { testErrorMessage } from "./testing-status";
import { Spinner } from "@/components/ui/spinner";
import { Badge } from "@/components/ui/badge";
import {
  catalogMessage,
  useCatalogMutation,
} from "@/components/catalog/mutation";

function accepted(value: unknown, provider: string) {
  if (!value || typeof value !== "object") return false;
  const receipt = value as API.CredentialTestAccepted;
  return (
    receipt.accepted === true &&
    receipt.provider_id === provider &&
    !!receipt.check_id &&
    !!receipt.model_id
  );
}

export function ConnectionOnboarding({
  id,
  revision = 0,
  modelID,
  adapter,
  selector,
  checks,
  onChecked,
}: {
  id: string;
  revision?: number;
  modelID?: string;
  adapter?: string;
  selector?: ReactNode | ((locked: boolean) => ReactNode);
  checks?: API.ProviderCheck[];
  onChecked?: () => void;
}) {
  const consoleURL = providerConsoleURL(adapter);
  const fieldID = useId();
  const prepareTrigger = useRef<HTMLButtonElement>(null);
  const session = useSession();
  const [requestedCheck, setRequestedCheck] = useState<{
    id: string;
    credentialID: string;
  } | null>(null);
  const [selected, setSelected] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [investigate, setInvestigate] = useState(false);
  const [note, setNote] = useState("");
  const { data, error, isLoading, mutate } = useSWR(
    ["provider-onboarding", session.actor.id, id, revision],
    () => getProviderOnboarding({ id }),
    {
      refreshInterval: (state) =>
        requestedCheck ||
        ["pending", "running"].includes(state?.check?.status ?? "")
          ? 2500
          : 0,
      revalidateOnFocus: true,
      onSuccess: (state) => {
        if (
          state.check?.id === requestedCheck?.id ||
          state.credential_id !== requestedCheck?.credentialID
        )
          setRequestedCheck(null);
      },
    },
  );
  const test = useCatalogMutation(
    (value) => {
      const receipt = value as API.CredentialTestAccepted;
      setRequestedCheck({
        id: receipt.check_id!,
        credentialID: data?.credential_id ?? "",
      });
      setSelected(receipt.model_id!);
      void mutate();
      onChecked?.();
    },
    `provider.onboarding.test.${id}`,
    accepted,
  );
  const query = useCatalogMutation(
    () => {
      void mutate();
    },
    `provider.onboarding.query.${id}`,
    accepted,
  );
  const endTracking = useCatalogMutation(() => {
    setInvestigate(false);
    setNote("");
    void mutate();
  }, `provider.onboarding.end.${id}`);
  const check = data?.check;
  const model =
    (data?.models?.some((m) => m.id === modelID) ? modelID : "") ||
    (data?.models?.some((m) => m.id === selected) ? selected : "") ||
    check?.model_id ||
    data?.models?.find((m) => m.capability === "text.generate")?.id ||
    data?.models?.[0]?.id ||
    "";
  const running = ["pending", "running"].includes(check?.status ?? "");
  const modelVersion = data?.models?.find((m) => m.id === model)?.version_id;
  const selectedCheck =
    check?.model_id === model &&
    !!modelVersion &&
    check.model_version_id === modelVersion
      ? check
      : checks?.find(
          (c) =>
            c.model_id === model &&
            c.model_version_id === modelVersion &&
            c.kind === "model_access" &&
            c.current,
        );
  const verified =
    selectedCheck?.status === "completed" &&
    selectedCheck.auth_result === "ok" &&
    ["completed", "accepted"].includes(selectedCheck.model_result ?? "");
  const awaitingCheck =
    !!requestedCheck &&
    requestedCheck.credentialID === data?.credential_id &&
    data?.check?.id !== requestedCheck.id;
  const waiting = running || awaitingCheck;
  const unresolved =
    (check?.model_result === "unknown" && !check.tracking_ended_at) ||
    (selectedCheck?.model_result === "unknown" &&
      !selectedCheck.tracking_ended_at);
  const busy = test.busy || query.busy || endTracking.busy || waiting;
  if (isLoading)
    return (
      <p role="status" className="flex items-center gap-2 text-sm">
        <Spinner />
        正在读取模型验证状态…
      </p>
    );
  if (error)
    return (
      <Alert variant="destructive">
        <AlertTitle>无法读取模型验证状态</AlertTitle>
        <AlertDescription>
          {catalogMessage(error)}
          <Button type="button" variant="outline" onClick={() => void mutate()}>
            重试
          </Button>
        </AlertDescription>
      </Alert>
    );
  if (!data?.models?.length) return null;
  const result = !data.credential_id
    ? "未配置"
    : test.busy || waiting
      ? "验证中"
      : test.pending || selectedCheck?.model_result === "unknown"
        ? "待确认"
        : verified
          ? selectedCheck?.model_result === "accepted"
            ? "已受理"
            : "已通过"
          : selectedCheck?.status === "failed"
            ? "未通过"
            : "未验证";
  const investigationCheck =
    selectedCheck?.model_result === "unknown" ? selectedCheck : check;
  const successMessages = [
    "已保存。",
    "原请求已保存，已恢复当前配置。",
    "请求已有成功回执，已刷新当前配置。",
    "已找到成功回执，列表已刷新。",
  ];
  function verify() {
    if (!data?.credential_id || !model || busy || unresolved) return;
    void test.run("credential.test", id, (requestID) =>
      testProviderOnboardingModel(
        { id, credential_id: data.credential_id!, model_id: model },
        { headers: { "Idempotency-Key": requestID } },
      ),
    );
  }

  return (
    <div className="flex min-w-0 flex-col gap-3">
      <Dialog
        open={confirmation === model && !!model}
        onOpenChange={(open) => {
          if (!open) setConfirmation("");
        }}
      >
        <DialogContent
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            prepareTrigger.current?.focus();
          }}
          className="sm:max-w-[480px]"
        >
          <DialogHeader>
            <DialogTitle>
              验证 {data.models.find((m) => m.id === model)?.display_name}
            </DialogTitle>
            <DialogDescription>费用未估算，按实际用量计费。</DialogDescription>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            {data.models.find((m) => m.id === model)?.capability ===
            "image.generate"
              ? "生成 1 张图片，尺寸按当前模型验证模板。"
              : data.models.find((m) => m.id === model)?.capability ===
                  "video.generate"
                ? "生成 1 个视频，时长与分辨率按当前模型验证模板。"
                : data.models.find((m) => m.id === model)?.capability ===
                    "audio.generate"
                  ? "使用短文本和当前模型合法音色验证。"
                  : "文本模板：请只回复 OK。输出上限按当前模型验证模板。"}
          </p>
          <p className="text-sm text-muted-foreground">
            结果未知时不自动重试。
          </p>
          {check?.tracking_ended_at ? (
            <p className="text-sm text-destructive">
              旧请求费用尚未确认，重新验证可能再次收费。
            </p>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmation("")}>
              取消
            </Button>
            <Button
              disabled={busy || !!test.pending || unresolved}
              onClick={() => {
                setConfirmation("");
                verify();
              }}
            >
              确认并验证
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={investigate} onOpenChange={setInvestigate}>
        <DialogContent className="sm:max-w-[480px]">
          <DialogHeader>
            <DialogTitle>核查原验证请求</DialogTitle>
            <DialogDescription>
              结果未知。请到服务商控制台核对请求或账单。
            </DialogDescription>
          </DialogHeader>
          <p className="text-xs break-all text-muted-foreground">
            请求标识：{investigationCheck?.request_id ?? "历史记录未提供"}
            <br />
            发起时间：
            {investigationCheck?.created_at
              ? new Date(investigationCheck.created_at).toLocaleString()
              : "未知"}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              className="max-md:h-11"
              disabled={!investigationCheck?.request_id}
              onClick={async () => {
                try {
                  if (await copy(investigationCheck!.request_id!))
                    toast.success("请求标识已复制");
                  else toast.error("未能复制，请手动选择请求标识");
                } catch {
                  toast.error("未能复制，请手动选择请求标识");
                }
              }}
            >
              复制请求标识
            </Button>
            {consoleURL ? (
              <Button variant="outline" asChild className="max-md:h-11">
                <a href={consoleURL} target="_blank" rel="noreferrer">
                  服务商控制台
                </a>
              </Button>
            ) : null}
          </div>
          <Field>
            <FieldLabel htmlFor={`${fieldID}-note`}>核查说明</FieldLabel>
            <Textarea
              id={`${fieldID}-note`}
              value={note}
              onChange={(e) => setNote(e.target.value)}
              maxLength={1000}
              placeholder="记录核查结论，请勿填写密钥或私有链接"
            />
          </Field>
          <p className="text-xs text-muted-foreground">
            结束追踪仍保留未知，不释放预算、不启用模型。新验证可能再次收费；需要对账的任务仍须先完成对账。
          </p>
          {endTracking.message ? (
            <p role="status" className="text-sm">
              {endTracking.message}
            </p>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setInvestigate(false)}>
              返回
            </Button>
            <Button
              variant="outline"
              disabled={
                !note.trim() || endTracking.busy || !!endTracking.pending
              }
              onClick={() =>
                void endTracking.run(
                  "provider.check.end_tracking",
                  investigationCheck!.id!,
                  (requestID) =>
                    endProviderCheckTracking(
                      { id: investigationCheck!.id! },
                      { request_id: requestID, note: note.trim() },
                    ),
                )
              }
            >
              结束追踪并保留未知
            </Button>
            {endTracking.pending ? (
              <Button
                variant="outline"
                disabled={endTracking.busy}
                onClick={endTracking.recover}
              >
                查询回执
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Field>
        <div className="flex items-center justify-between gap-2">
          <FieldLabel htmlFor={`${fieldID}-model`}>模型验证</FieldLabel>
          <Badge
            variant={result === "未通过" ? "destructive" : "secondary"}
            role="status"
          >
            {result === "验证中" ? (
              <Spinner data-icon="inline-start" />
            ) : result === "已通过" || result === "已受理" ? (
              <Check data-icon="inline-start" />
            ) : null}
            {result}
          </Badge>
        </div>
        <div className="flex min-w-0 items-center gap-2">
          {(typeof selector === "function"
            ? selector(busy || unresolved || !!test.pending || !!query.pending)
            : selector) ?? (
            <Select
              value={model}
              disabled={busy || unresolved || !!test.pending || !!query.pending}
              onValueChange={setSelected}
            >
              <SelectTrigger
                id={`${fieldID}-model`}
                className="min-w-0 flex-1 max-md:min-h-11"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {data.models.map((m) =>
                    m.id ? (
                      <SelectItem key={m.id} value={m.id}>
                        {m.display_name}
                      </SelectItem>
                    ) : null,
                  )}
                </SelectGroup>
              </SelectContent>
            </Select>
          )}
          {test.pending ? (
            <Button
              type="button"
              variant="outline"
              disabled={test.busy}
              onClick={test.recover}
              className="shrink-0 max-md:h-11"
            >
              {test.busy ? <Spinner data-icon="inline-start" /> : null}
              {test.busy ? "提交中…" : "查询回执"}
            </Button>
          ) : query.pending ? (
            <Button
              variant="outline"
              className="shrink-0 max-md:h-11"
              disabled={query.busy}
              onClick={query.recover}
            >
              查询回执
            </Button>
          ) : waiting ? null : selectedCheck?.task_id &&
            ["accepted", "unknown"].includes(
              selectedCheck.model_result ?? "",
            ) ? (
            <Button
              variant="outline"
              className="shrink-0 max-md:h-11"
              disabled={busy}
              onClick={() =>
                void query.run("credential.test", id, (requestID) =>
                  queryProviderModelCheck(
                    { id: selectedCheck.id! },
                    { headers: { "Idempotency-Key": requestID } },
                  ),
                )
              }
            >
              查询原任务
            </Button>
          ) : unresolved ? (
            <Button
              variant="outline"
              className="shrink-0 max-md:h-11"
              onClick={() => setInvestigate(true)}
            >
              查看核查说明
            </Button>
          ) : (
            <Button
              type="button"
              variant="outline"
              disabled={busy || !data.credential_id || !model}
              className="shrink-0 max-md:h-11"
              ref={prepareTrigger}
              onClick={() => {
                if (check?.tracking_ended_at) setConfirmation(model);
                else verify();
              }}
            >
              {test.busy ? <Spinner data-icon="inline-start" /> : null}
              验证
            </Button>
          )}
        </div>
        <FieldDescription>
          使用所选模型的最小模板，供应商可能按实际用量收费。
        </FieldDescription>
        {test.pending && !test.busy ? (
          <FieldDescription>先查询原回执，避免重复调用。</FieldDescription>
        ) : null}
        {selectedCheck?.completed_at ? (
          <Collapsible>
            <CollapsibleTrigger asChild>
              <Button
                variant="ghost"
                className="h-auto self-start px-0 text-xs text-muted-foreground"
              >
                验证详情
              </Button>
            </CollapsibleTrigger>
            <CollapsibleContent>
              <FieldDescription>
                {selectedCheck.task_id ? "提交受理耗时" : "总耗时"}：
                {selectedCheck.elapsed_ms ?? 0} ms ·{" "}
                {new Date(selectedCheck.completed_at).toLocaleString()}
              </FieldDescription>
            </CollapsibleContent>
          </Collapsible>
        ) : null}
        {verified &&
        selectedCheck?.model_result === "accepted" &&
        !busy &&
        !test.pending ? (
          <FieldDescription>仍在生成，可查询原任务。</FieldDescription>
        ) : null}
      </Field>
      {selectedCheck?.status === "failed" && !busy && !test.pending ? (
        <Alert
          variant={
            selectedCheck?.model_result === "unknown"
              ? "default"
              : "destructive"
          }
        >
          <AlertDescription>
            {testErrorMessage(selectedCheck.auth_result ?? "")}
          </AlertDescription>
        </Alert>
      ) : null}
      {!test.pending &&
      test.message &&
      !successMessages.includes(test.message) ? (
        <Alert variant="destructive">
          <AlertDescription>{test.message}</AlertDescription>
        </Alert>
      ) : null}
      {verified ? (
        <p className="text-xs text-muted-foreground">
          此回执仅记录当前模型的接入检查。请在发布复核中查看逐模式证据。
        </p>
      ) : null}
      {query.message && query.message !== "已保存。" ? (
        <p role="status" className="text-sm">
          {query.message}
        </p>
      ) : null}
    </div>
  );
}
