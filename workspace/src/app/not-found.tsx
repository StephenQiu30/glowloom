import Link from "next/link";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from "@/components/ui/empty";
export default function NotFound() {
  return (
    <main id="main" className="page">
      <Empty>
        <EmptyHeader>
          <EmptyTitle>这篇文档不在当前版本中</EmptyTitle>
          <EmptyDescription>可以返回知识库，或按关键词搜索。</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button asChild variant="outline">
            <Link href="/search/">搜索知识库</Link>
          </Button>
        </EmptyContent>
      </Empty>
    </main>
  );
}
