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
        <h2 className="text-xl font-semibold">infinite-canvas</h2>
        <p className="mt-3 text-sm leading-7 text-muted-foreground">
          画布 PoC 的卡片外观与浮动工具栏交互参考了 basketikun 的
          infinite-canvas。 我们依据固定的 MIT
          许可版本重新实现界面，未采用其存储、插件或执行代码。
        </p>
        <div className="mt-5 flex flex-wrap gap-5 text-sm">
          <a
            href="https://github.com/basketikun/infinite-canvas/tree/dab19adc0847e32e39b7fc8ff90cb392561fb826"
            className="text-blue-600 underline-offset-4 hover:underline"
          >
            查看固定源码版本
          </a>
          <a
            href="/licenses/infinite-canvas.txt"
            className="text-blue-600 underline-offset-4 hover:underline"
          >
            阅读 MIT 许可全文
          </a>
        </div>
      </section>
    </main>
  );
}
