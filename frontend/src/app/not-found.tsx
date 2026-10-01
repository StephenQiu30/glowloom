import Link from "next/link";
import { Button } from "@/components/ui/button";

export default function NotFound() {
  return (
    <main className="flex min-h-dvh flex-col items-center justify-center gap-5">
      <h1 className="text-3xl font-medium">页面不存在</h1>
      <p className="text-muted-foreground">链接可能已失效，请返回工作区。</p>
      <Button asChild>
        <Link href="/">返回首页</Link>
      </Button>
    </main>
  );
}
