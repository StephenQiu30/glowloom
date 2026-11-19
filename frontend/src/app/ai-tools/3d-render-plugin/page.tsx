import Link from "next/link";
import { PageLayout } from "@/components/layout/page-layout";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardTitle } from "@/components/ui/card";

export const metadata = { title: "3D渲染插件 · 织光" };

export default function Page() {
  return (
    <PageLayout screen="ai-tools" hideSearch>
      <section className="mx-auto flex w-full max-w-3xl flex-col gap-6">
        <Button variant="ghost" asChild className="self-start">
          <Link href="/ai-tools">返回 AI工具</Link>
        </Button>
        <h1 className="text-3xl font-semibold">3D渲染插件</h1>
        <div className="grid gap-4 sm:grid-cols-2">
          {["Blender", "Maya"].map((host) => (
            <Card key={host}>
              <CardHeader>
                <CardTitle>{host} 插件</CardTitle>
                <Badge variant="secondary" className="mt-2 w-fit">
                  待接入
                </Badge>
              </CardHeader>
            </Card>
          ))}
        </div>
      </section>
    </PageLayout>
  );
}
