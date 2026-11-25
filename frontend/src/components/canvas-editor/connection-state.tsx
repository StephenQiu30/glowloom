import Link from "next/link";
import type { ReactNode } from "react";
import { ContentState } from "@/components/layout/content-state";
import { CanvasLoading } from "@/components/layout/loading-state";
import { Button } from "@/components/ui/button";

/** Presentation only: never converts an unconfirmed write into a success. */
export function canvasConnectionMessage(message: string): string {
  const messages: Record<string, string> = {
    unsupported_protocol_version: "画布服务版本不兼容",
    dependency_unavailable: "画布服务暂不可用",
    forbidden: "无权访问此画布",
    unauthorized: "登录已失效，请重新登录",
    not_found: "画布不存在或无权访问",
    network_error: "画布连接中断",
    MAIN_CANVAS_MANAGED_BY_PROJECT:
      "主画布名称由所属项目管理，请从画布菜单重新命名。已有输入保留在待恢复列表中。",
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
  if (!message) return <CanvasLoading message="正在连接画布…" />;

  return (
    <ContentState
      className="min-h-dvh"
      state="error"
      heading="h1"
      title={canvasConnectionMessage(message)}
      actions={
        <>
          {scopeControl && /forbidden|not_found|权限|无权|空间/.test(message)
            ? scopeControl
            : null}
          <Button asChild variant="outline" size="feedback">
            <Link href="/projects">返回画布列表</Link>
          </Button>
          {message !== "画布访问权限已失效" && (
            <Button size="feedback" onClick={onRetry}>
              重新连接
            </Button>
          )}
        </>
      }
    />
  );
}
