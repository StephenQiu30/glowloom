import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "开源许可 | Lanverse",
};

export default function LicensesPage() {
  return (
    <main className="mx-auto max-w-3xl px-6 py-16 text-foreground">
      <Link
        href="/projects"
        className="text-sm text-muted-foreground hover:text-foreground"
      >
        ← 返回 Lanverse
      </Link>
      <h1 className="mt-10 text-3xl font-semibold tracking-tight">开源许可</h1>
      <section className="mt-10 rounded-2xl border border-border p-6 sm:p-8">
        <h2 className="text-xl font-semibold">BeefTV · Infinite Canvas</h2>
        <p className="mt-3 text-sm leading-7 text-muted-foreground">
          Lanverse 的无限画布移植自 BeefTV 的 MIT 许可版本，复用其视口、选择、
          布局、分组、连线和浮动工具设计，并接入 Lanverse 的身份与项目数据服务。
          BeefTV 包含源自 basketikun 的 Infinite Canvas 代码。
        </p>
        <div className="mt-5 flex flex-wrap gap-5 text-sm">
          <a
            href="https://github.com/glanderness/BeefTV/tree/0d9e9f48d407570cd431ad9730cdd522b06810c0"
            className="text-blue-600 underline-offset-4 hover:underline"
          >
            查看固定源码版本
          </a>
          <a
            href="/licenses/beeftv.txt"
            className="text-blue-600 underline-offset-4 hover:underline"
          >
            阅读 MIT 许可全文
          </a>
        </div>
      </section>
    </main>
  );
}
