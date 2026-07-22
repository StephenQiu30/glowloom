import Link from "next/link";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default function Home() {
  return (
    <main
      id="main-content"
      className="mx-auto flex min-h-[calc(100vh-4rem)] max-w-6xl items-center px-6 py-16"
    >
      <Card className="w-full">
        <CardHeader>
          <CardDescription>Thief</CardDescription>
          <CardTitle className="text-4xl sm:text-5xl">
            <h1>发现灵感，开始创作</h1>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <p className="max-w-2xl text-base leading-7 text-muted-foreground">
            从优质提示词与示例中获得灵感，再进入工作台完成自己的图片创作。
          </p>
          <div className="flex flex-wrap gap-3">
            <Button asChild size="lg">
              <Link href="/explore">探索灵感</Link>
            </Button>
            <Button asChild size="lg" variant="outline">
              <Link href="/create">空白创作</Link>
            </Button>
          </div>
        </CardContent>
      </Card>
    </main>
  );
}
