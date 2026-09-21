"use client";

import { LayoutContainer } from "@/layout/layout-container";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";

export default function ErrorPage({
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <LayoutContainer className="flex min-h-80 flex-col justify-center gap-5 py-10">
      <Alert variant="destructive">
        <AlertTitle>页面暂时无法显示</AlertTitle>
        <AlertDescription>请重试加载。已经保存的项目内容不会因此丢失。</AlertDescription>
      </Alert>
      <Button onClick={reset} className="w-fit">
        重新加载
      </Button>
    </LayoutContainer>
  );
}
