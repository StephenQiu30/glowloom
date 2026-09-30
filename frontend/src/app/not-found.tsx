import Link from "next/link";
import { Button } from "@/components/ui/button";
export default function NotFound() {
  return (
    <div className="mx-auto flex min-h-[60vh] max-w-md flex-col items-center justify-center gap-5 px-6 text-center">
      <p className="font-mono text-sm text-muted-foreground">404</p>
      <h1 className="text-3xl font-semibold">页面或对象不存在</h1>
      <p className="text-sm leading-7 text-muted-foreground">
        请检查项目、单集、镜头或任务链接，或者回到工作台。
      </p>
      <Button asChild>
        <Link href="/projects">返回项目</Link>
      </Button>
    </div>
  );
}
