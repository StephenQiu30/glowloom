import Link from "next/link";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default function NotFound() {
  return (
    <main
      id="main-content"
      className="mx-auto flex min-h-[calc(100vh-4rem)] max-w-3xl items-center px-6 py-16"
    >
      <Card className="w-full">
        <CardHeader>
          <CardTitle>
            <h1>页面不存在</h1>
          </CardTitle>
          <CardDescription>你访问的内容可能已被移动。</CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild>
            <Link href="/">返回首页</Link>
          </Button>
        </CardContent>
      </Card>
    </main>
  );
}
