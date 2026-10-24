import Link from "next/link";
import type { ReactNode } from "react";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";

/** Presentation only: never converts an unconfirmed write into a success. */
export function canvasConnectionMessage(message: string): string {
  const messages: Record<string, string> = {
    unsupported_protocol_version:
      "画布服务版本不兼容，请稍后重试或返回画布列表。",
    dependency_unavailable: "画布服务暂时不可用，请稍后重试。",
    forbidden: "当前账号没有访问此画布的权限。",
    unauthorized: "登录已失效，请重新登录后打开画布。",
    not_found: "画布不存在或当前账号无法访问。",
    network_error: "连接已中断，请检查网络后重试。",
  };
  if (messages[message]) return messages[message];
  // Locally authored recovery instructions retain their precise save semantics.
  if (
    /[\u3400-\u9fff]/.test(message) &&
    !/[a-z]+_[a-z]+|https?:\/\//i.test(message)
  )
    return message;
  return "画布连接暂不可用，保存结果尚未确认。请保留此页面并重新连接。";
}

export function CanvasConnectionState({
  message,
  onRetry,
  scopeControl,
}: {
  message: string;
  onRetry: () => void;
  scopeControl?: ReactNode;
}) {
  return (
    <ContentState
      className="min-h-dvh"
      state={message ? "error" : "loading"}
      title={message ? "暂时无法打开画布" : "正在连接画布"}
      description={
        message ? canvasConnectionMessage(message) : "正在连接你的创作空间。"
      }
      actions={
        <>
          {scopeControl &&
            /forbidden|not_found|权限|无权|空间/.test(message) && (
              <div className="flex basis-full justify-center pb-2">
                {scopeControl}
              </div>
            )}
          <Button asChild variant="secondary">
            <Link href="/projects">返回画布列表</Link>
          </Button>
          {message && message !== "画布访问权限已失效" && (
            <Button onClick={onRetry}>重新连接</Button>
          )}
        </>
      }
    />
  );
}
