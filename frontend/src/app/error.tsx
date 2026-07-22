"use client";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default function ErrorPage({ reset }: { reset: () => void }) {
  return (
    <main
      id="main-content"
      className="mx-auto flex min-h-[calc(100vh-4rem)] max-w-3xl items-center px-6 py-16"
    >
      <Card className="w-full">
        <CardHeader>
          <CardTitle>
            <h1>页面暂时不可用</h1>
          </CardTitle>
          <CardDescription>请稍后重试。</CardDescription>
        </CardHeader>
        <CardContent>
          <Button onClick={reset}>重新加载</Button>
        </CardContent>
      </Card>
    </main>
  );
}
