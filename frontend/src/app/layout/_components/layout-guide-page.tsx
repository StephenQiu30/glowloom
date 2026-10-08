import { cn } from "cn";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";

function LayoutDiagram({
  collapsed = false,
  compact = false,
}: {
  collapsed?: boolean;
  compact?: boolean;
}) {
  return (
    <div
      className={cn(
        "flex overflow-hidden rounded-lg bg-background",
        compact ? "h-[120px]" : "h-[480px]",
      )}
    >
      <div className="flex w-[72px] shrink-0 flex-col items-center gap-5 bg-surface-1 py-4">
        {!compact ? <span className="size-8 rounded-md bg-primary" /> : null}
        {!compact
          ? ["项目", "素材", "任务", "管理"].map((label) => (
              <span className="text-[10px] text-muted-foreground" key={label}>
                {label}
              </span>
            ))
          : null}
      </div>
      {!collapsed ? (
        <div className="hidden w-[220px] shrink-0 flex-col gap-4 bg-surface-1 px-4 py-5 text-xs sm:flex">
          {!compact ? <strong>雾港来信</strong> : null}
          {!compact
            ? [
                "概览",
                "剧本",
                "设定集",
                "第 3 集",
                "资产定稿",
                "分镜",
                "配音",
                "画布",
              ].map((label) => (
                <span
                  key={label}
                  className={cn(
                    label === "分镜"
                      ? "rounded-md bg-surface-3 p-2"
                      : "p-2 text-muted-foreground",
                  )}
                >
                  {label}
                </span>
              ))
            : null}
        </div>
      ) : null}
      <div className="flex min-w-0 flex-1 flex-col p-3 sm:p-6">
        {!compact ? (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 className="text-sm font-medium">页面标题 24px</h3>
              <p className="mt-2 text-xs text-muted-foreground">一页一主标题</p>
            </div>
            <Button size="sm">主操作</Button>
          </div>
        ) : null}
        {!compact ? (
          <span className="m-auto text-xs text-muted-foreground">
            内容 · max-width 1280 · 左右 24px
          </span>
        ) : null}
      </div>
    </div>
  );
}

export function LayoutGuidePage() {
  return (
    <main className="mx-auto flex w-full max-w-[1440px] min-w-0 flex-col gap-12 px-6 py-16 lg:px-20 [&_[data-slot=card]]:[--card-spacing:--spacing(4)]">
      <header>
        <p className="font-mono text-xs text-muted-foreground">
          LAYOUT · v2 · 侧边布局
        </p>
        <h1 className="mt-3 text-[40px] leading-[48px] font-semibold">
          页框与导航
        </h1>
        <p className="mt-4 max-w-[720px] text-sm leading-6 text-muted-foreground">
          全站统一使用左侧主导航，无顶部栏。主导航宽 72px
          保持全局入口；进入有上下文的页面后，右侧展开 220px
          的二级导航。分清内容和导航的职责，内容从标题开始。
        </p>
      </header>
      <section>
        <h2 className="mb-5 text-lg font-medium">页框结构</h2>
        <Card variant="panel">
          <CardContent>
            <LayoutDiagram />
            <p className="mt-4 text-xs leading-6 text-muted-foreground">
              ① 主导航 72px：品牌与全局创建、项目、素材、任务、管理。② 二级导航
              220px：项目或管理上下文。③
              内容区：页面主标题、工具栏、主操作与页面内容。
            </p>
          </CardContent>
        </Card>
      </section>
      <section>
        <h2 className="mb-5 text-lg font-medium">每个页面的侧栏组合</h2>
        <div className="overflow-hidden rounded-xl bg-surface-1">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>页面</TableHead>
                <TableHead>主导航</TableHead>
                <TableHead>二级导航</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {[
                ["首页与项目库", "项目", "无"],
                ["素材库", "素材", "个人 / 项目与文件夹"],
                ["概览、设定集、分镜、配音", "项目", "项目导航"],
                ["画布", "无常驻", "浮动工具栏与面包屑"],
                ["镜头详情", "项目", "无"],
                ["管理页面", "管理", "管理导航"],
                ["登录、注册、改密", "无", "无"],
              ].map((row) => (
                <TableRow key={row[0]}>
                  {row.map((value, index) => (
                    <TableCell key={index} className="py-3">
                      {value}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </section>
      <section>
        <h2 className="mb-5 text-lg font-medium">收起侧栏</h2>
        <div className="grid gap-5 lg:grid-cols-2">
          <Card variant="panel">
            <CardHeader>
              <CardTitle>展开 · 默认</CardTitle>
            </CardHeader>
            <CardContent>
              <LayoutDiagram compact />
              <p className="mt-4 text-xs text-muted-foreground">
                主轨 72px，二级展开 220px。
              </p>
            </CardContent>
          </Card>
          <Card variant="panel">
            <CardHeader>
              <CardTitle>收起</CardTitle>
            </CardHeader>
            <CardContent>
              <LayoutDiagram collapsed compact />
              <p className="mt-4 text-xs text-muted-foreground">
                主轨始终可见；保留上下文并扩大内容区。
              </p>
            </CardContent>
          </Card>
        </div>
      </section>
      <section>
        <h2 className="mb-4 text-lg font-medium">窄屏</h2>
        <p className="text-sm leading-7 text-muted-foreground">
          窄屏下主导航和上下文导航收入可关闭抽屉。内容区卡片按宽度重排，保留主操作与当前页面标题。画布保持独立可滚动视口。
        </p>
      </section>
    </main>
  );
}
