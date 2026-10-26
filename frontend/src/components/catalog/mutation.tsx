"use client";
import { pageBusinessMemory } from "@/lib/page-business-memory";

import { FeedbackToast } from "@/components/feedback/feedback-toast";

import { useRef, useState, useSyncExternalStore } from "react";
import { useSession } from "@/components/auth/session";
import { RequestError } from "@/lib/request";
import { getConfigReceipt } from "@/api/settings";
import { Button } from "@/components/ui/button";

export const zeroID = "00000000-0000-0000-0000-000000000000";
type Pending = { id: string; action: string; target: string };
export function catalogMessage(error: unknown) {
  if (error instanceof RequestError) {
    const messages: Record<string, string> = {
      forbidden: "仅平台管理员可以管理平台连接和模型。",
      revision_conflict: "配置已变化或请求标识已使用，请刷新并核对回执。",
      preset_unavailable: "预设已归档或被紧急停用，请重新选择。",
      release_unverified: "发布尚未完成全部模式的真实验证，暂不能激活。",
      catalog_unavailable: "连接、模型或价格条件未满足，请检查配置。",
      invalid_preset: "预设格式不正确，或包含不允许的字段及端点。",
      invalid_request: "请检查必填项、参数类型和版本号。",
      dependency_unavailable: "服务暂不可用，请检查后台配置后重试。",
      credential_sealing_unavailable:
        "服务端尚未配置凭据加密，API Key 未保存。请配置加密密钥后重试。",
      request_unconfirmed: "请求结果未确认。请先查询回执，避免重复写入。",
    };
    return messages[error.code] ?? error.message;
  }
  return error instanceof Error ? error.message : "操作失败，请重试。";
}

// Request identities remain in page memory. Browser business records are never read.
export function useCatalogMutation(
  onSaved: (result?: unknown) => void,
  scope = "registry",
  validateResult?: (result: unknown, target: string) => boolean,
) {
  const session = useSession();
  const storageKey = `glowloom.catalog.pending.v1.${session.actor.id}.${scope}`;
  const snapshot = useSyncExternalStore(
    subscribePending,
    () => readPending(storageKey),
    () => "server",
  );
  const pending = parsePending(snapshot);
  const ready = snapshot !== "server" && snapshot !== "unavailable";
  const [busy, setBusy] = useState(false);
  const writeLock = useRef(false);
  const [message, setMessage] = useState("");
  function clear() {
    pageBusinessMemory.removeItem(storageKey);
    window.dispatchEvent(new Event(pendingEvent));
  }
  async function run<T>(
    action: string,
    target: string,
    execute: (id: string) => Promise<T>,
  ): Promise<T | undefined> {
    if (!ready || writeLock.current) return;
    if (pending && (pending.action !== action || pending.target !== target)) {
      setMessage("有尚未确认的配置请求，请先查询其回执。");
      return;
    }
    const request = pending ?? { id: crypto.randomUUID(), action, target };
    writeLock.current = true;
    setBusy(true);
    setMessage("");
    try {
      pageBusinessMemory.setItem(storageKey, JSON.stringify(request));
      window.dispatchEvent(new Event(pendingEvent));
      if (pending) {
        try {
          const receipt = await getConfigReceipt({
            id: request.id,
            action: request.action,
            target_id: request.target,
          });
          if (validateResult && !validateResult(receipt.result, target))
            throw new RequestError("request_unconfirmed");
          clear();
          setMessage("原请求已保存，已恢复当前配置。");
          onSaved(receipt.result);
          return;
        } catch (error) {
          if (!(error instanceof RequestError && error.status === 404))
            throw error;
        }
      }
      const result = await execute(request.id);
      if (validateResult && !validateResult(result, target))
        throw new RequestError("request_unconfirmed");
      clear();
      setMessage("已保存。");
      onSaved(result);
      return result;
    } catch (error) {
      if (
        error instanceof RequestError &&
        error.status >= 400 &&
        error.status < 500 &&
        error.status !== 409
      )
        clear();
      setMessage(catalogMessage(error));
      if (error instanceof RequestError && error.status === 409) {
        try {
          const receipt = await getConfigReceipt({
            id: request.id,
            action: request.action,
            target_id: request.target,
          });
          if (validateResult && !validateResult(receipt.result, request.target))
            throw new RequestError("request_unconfirmed");
          clear();
          setMessage("请求已有成功回执，已刷新当前配置。");
          onSaved(receipt.result);
        } catch (receiptError) {
          if (
            receiptError instanceof RequestError &&
            receiptError.status === 404
          ) {
            clear();
            setMessage("本次变更未保存，当前配置已变化。请刷新后重新编辑。");
          }
        }
      }
    } finally {
      writeLock.current = false;
      setBusy(false);
    }
  }
  async function recover() {
    if (!pending || writeLock.current) return;
    writeLock.current = true;
    setBusy(true);
    try {
      const receipt = await getConfigReceipt({
        id: pending.id,
        action: pending.action,
        target_id: pending.target,
      });
      if (validateResult && !validateResult(receipt.result, pending.target))
        throw new RequestError("request_unconfirmed");
      clear();
      setMessage("已找到成功回执，列表已刷新。");
      onSaved(receipt.result);
    } catch (error) {
      setMessage(
        error instanceof RequestError && error.status === 404
          ? "尚未找到成功回执。请用原表单重试；Key 不会被保存，需重新输入相同 Key。"
          : catalogMessage(error),
      );
    } finally {
      writeLock.current = false;
      setBusy(false);
    }
  }
  return {
    run,
    busy: busy || !ready,
    message:
      !ready && snapshot === "unavailable"
        ? "请允许此站点使用浏览器存储，以保留请求标识并恢复未确认的操作。Key 不会写入浏览器存储。"
        : message,
    pending,
    recover,
  };
}
export function MutationStatus({
  mutation,
  inline = false,
}: {
  mutation: ReturnType<typeof useCatalogMutation>;
  inline?: boolean;
}) {
  return (
    <div className="space-y-2" aria-live="polite">
      {mutation.message ? (
        inline ? (
          <p role="status" className="text-sm break-words">
            {mutation.message}
          </p>
        ) : (
          <FeedbackToast message={mutation.message} tone="info" />
        )
      ) : null}
      {mutation.pending ? (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border p-3 text-sm">
          <span>待确认请求：{mutation.pending.action}</span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={mutation.busy}
            onClick={mutation.recover}
          >
            查询回执
          </Button>
        </div>
      ) : null}
    </div>
  );
}

const pendingEvent = "glowloom:catalog-pending";
function subscribePending(onChange: () => void) {
  window.addEventListener("storage", onChange);
  window.addEventListener(pendingEvent, onChange);
  return () => {
    window.removeEventListener("storage", onChange);
    window.removeEventListener(pendingEvent, onChange);
  };
}
function readPending(key: string) {
  try {
    return pageBusinessMemory.getItem(key) ?? "";
  } catch {
    return "unavailable";
  }
}
function parsePending(raw: string): Pending | null {
  try {
    const value: unknown = JSON.parse(raw);
    if (
      value &&
      typeof value === "object" &&
      "id" in value &&
      "action" in value &&
      "target" in value &&
      typeof value.id === "string" &&
      typeof value.action === "string" &&
      typeof value.target === "string"
    )
      return { id: value.id, action: value.action, target: value.target };
  } catch {}
  return null;
}
