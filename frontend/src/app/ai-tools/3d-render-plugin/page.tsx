import Link from "next/link";
import { PageHeader } from "@/components/layout/page-header";
import { PageLayout } from "@/components/layout/page-layout";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardTitle } from "@/components/ui/card";

export const metadata = { title: "3D渲染插件 · 织光" };

export default function Page() {
  return (
    <PageLayout screen="ai-tools" hideSearch>
      <section className="flex w-full max-w-3xl flex-col gap-(--layout-gap)">
        <PageHeader title="3D渲染插件">
          <Button variant="ghost" asChild>
            <Link href="/ai-tools">返回 AI工具</Link>
          </Button>
        </PageHeader>
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
