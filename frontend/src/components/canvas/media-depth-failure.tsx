import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ApiError } from "@/lib/request";
export function DepthFailure({
  title,
  error,
  retry,
}: {
  title: string;
  error: unknown;
  retry?: () => void;
}) {
  return (
    <Alert variant="destructive">
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>
        {error instanceof Error ? error.message : "请求尚未完成。"}
        {error instanceof ApiError && error.requestId ? (
          <p>请求编号：{error.requestId}</p>
        ) : null}
      </AlertDescription>
      {retry ? (
        <Button variant="ghost" onClick={retry}>
          重试读取
        </Button>
      ) : null}
    </Alert>
  );
}
