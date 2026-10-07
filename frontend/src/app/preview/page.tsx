import Link from "next/link";
import { ArrowRight, ExternalLink } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { previewHref, previewScreens } from "@/components/preview/screens";

export const metadata = { title: "全部设计画板 · 本地演示" };
export default function PreviewIndex() {
  return (
    <main className="mx-auto flex max-w-6xl flex-col gap-10 px-6 py-14">
      <header className="flex flex-wrap items-start justify-between gap-6">
        <div>
          <Badge variant="secondary">本地 mock · Figma 视觉预览</Badge>
          <h1 className="mt-5 text-3xl font-semibold tracking-tight">
            浮光 · 让每个故事成片
          </h1>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">
            全部 18
            个设计画板。数据与操作仅用于演示，刷新恢复，不连接真实账号、媒体或生成服务。
          </p>
        </div>
        <Button asChild>
          <Link href={previewHref("home")}>
            进入工作台
            <ArrowRight data-icon="inline-end" />
          </Link>
        </Button>
      </header>
      {["创作", "账号", "管理", "规范"].map((group) => (
        <section key={group}>
          <h2 className="mb-4 text-lg font-medium">{group}</h2>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {previewScreens
              .filter((item) => item.group === group)
              .map((item) => (
                <Card key={item.id}>
                  <CardHeader>
                    <CardTitle>
                      <Link
                        href={previewHref(item.id)}
                        className="hover:underline"
                      >
                        {item.name}
                      </Link>
                    </CardTitle>
                    <CardDescription>Figma · {item.node}</CardDescription>
                  </CardHeader>
                  <CardContent className="flex gap-2">
                    <Button asChild variant="secondary">
                      <Link href={previewHref(item.id)}>
                        查看页面
                        <ArrowRight data-icon="inline-end" />
                      </Link>
                    </Button>
                    <Button asChild variant="ghost" size="icon">
                      <a
                        href={`https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT/?node-id=${item.node.replace(":", "-")}`}
                        target="_blank"
                        rel="noreferrer"
                        aria-label={`在 Figma 查看${item.name}`}
                      >
                        <ExternalLink data-icon="inline-start" />
                      </a>
                    </Button>
                  </CardContent>
                </Card>
              ))}
          </div>
        </section>
      ))}
      <Button asChild variant="link" className="self-start">
        <Link href="/">返回真实业务工作台</Link>
      </Button>
    </main>
  );
}
