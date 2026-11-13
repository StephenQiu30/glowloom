import Link from "next/link";
import type { ReactNode } from "react";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Skeleton } from "@/components/ui/skeleton";
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
  return (
    <div className="relative flex min-h-dvh items-end justify-center p-6">
      <Skeleton
        className="absolute inset-0 rounded-none"
        role="status"
        aria-label="正在连接画布"
        aria-busy={!message}
      />
      <FeedbackToast
        message={message ? canvasConnectionMessage(message) : ""}
        action={
          message && message !== "画布访问权限已失效"
            ? { label: "重新连接", onClick: onRetry }
            : undefined
        }
      />
      <div className="relative flex items-center gap-2">
        {scopeControl &&
          /forbidden|not_found|权限|无权|空间/.test(message) &&
          scopeControl}
        <Button asChild variant="secondary">
          <Link href="/projects">返回画布列表</Link>
        </Button>
      </div>
    </div>
  );
}
