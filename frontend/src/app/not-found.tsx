import { ContentState } from "@/components/layout/content-state";
import Link from "next/link";
import { Button } from "@/components/ui/button";

export default function NotFound() {
  return (
    <main>
      <ContentState
        className="min-h-dvh"
        title="页面不存在"
        description="链接可能已失效，请返回工作区。"
        actions={
          <Button asChild>
            <Link href="/">返回首页</Link>
          </Button>
        }
      />
    </main>
  );
}
