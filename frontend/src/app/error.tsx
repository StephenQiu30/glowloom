"use client";
import Link from "next/link";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";

export default function PageError({ retry }: { retry: () => void }) {
  return (
    <ContentState
      state="error"
      title="暂时无法显示此页面"
      description="请重新加载内容，或返回工作台。"
      actions={
        <>
          <Button asChild variant="secondary">
            <Link href="/">返回工作台</Link>
          </Button>
          <Button onClick={retry}>重新加载</Button>
        </>
      }
    />
  );
}
