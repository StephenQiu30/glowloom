import Link from "next/link";
export default function LicensesPage() {
  return (
    <main className="min-h-svh p-6 md:p-10">
      <div className="max-w-2xl space-y-6">
        <Link href="/" className="text-sm text-muted-foreground">
          返回织光首页
        </Link>
        <h1 className="text-2xl font-medium">第三方许可</h1>
        <p className="leading-7 text-muted-foreground">
          当前画布适配 BeefTV 与 Infinite Canvas 的 MIT 许可代码。
          历史时间线算法保留 Lingji Cut 的 Apache-2.0 声明。
        </p>
        <ul className="space-y-3">
          <li>
            <a
              className="underline underline-offset-4"
              href="/licenses/beeftv.txt"
            >
              BeefTV MIT 许可
            </a>
          </li>
          <li>
            <a
              className="underline underline-offset-4"
              href="/licenses/beeftv-notice.txt"
            >
              BeefTV 上游来源与版权声明
            </a>
          </li>
          <li>
            <a
              className="underline underline-offset-4"
              href="/licenses/infinite-canvas.txt"
            >
              Infinite Canvas MIT 许可
            </a>
          </li>
          <li>
            <a
              className="underline underline-offset-4"
              href="/licenses/lingji-cut.txt"
            >
              Lingji Cut Apache-2.0 许可
            </a>
          </li>
          <li>
            <a
              className="underline underline-offset-4"
              href="/licenses/video-depth-anything.txt"
            >
              Video Depth Anything Apache-2.0 许可
            </a>
          </li>
          <li>
            <a
              className="underline underline-offset-4"
              href="/licenses/video-depth-anything-small.md"
            >
              Video Depth Anything Small 权重来源
            </a>
          </li>
          <li>
            <a
              className="underline underline-offset-4"
              href="/licenses/CC-BY-4.0.txt"
            >
              Khronos glTF 核心规范 CC-BY-4.0
            </a>
          </li>
          <li>
            <a
              className="underline underline-offset-4"
              href="/licenses/LicenseRef-KhronosSpecCopyright.txt"
            >
              Khronos 扩展规范版权声明
            </a>
          </li>
          <li>
            <a
              className="underline underline-offset-4"
              href="/licenses/noto-cjk-ofl.txt"
            >
              Noto CJK 字幕字体 SIL Open Font License 1.1
            </a>
          </li>
          <li>
            <a
              className="underline underline-offset-4"
              href="/licenses/geist-ofl.txt"
            >
              Geist 界面字体 SIL Open Font License 1.1
            </a>
          </li>
          <li>
            <a
              className="underline underline-offset-4"
              href="/licenses/geist-mono-ofl.txt"
            >
              Geist Mono 界面字体 SIL Open Font License 1.1
            </a>
          </li>
          <li>
            <a
              className="underline underline-offset-4"
              href="/licenses/noto-sans-sc-ofl.txt"
            >
              Noto Sans SC 界面字体 SIL Open Font License 1.1
            </a>
          </li>
        </ul>
        <p className="text-sm text-muted-foreground">
          当前画布来源：BeefTV a2196d3；历史来源为 0d9e9f48 / 1ae25027。© 2026
          @beefnoode、BeefTV contributors、basketikun、ddcat、yoqu。
        </p>
        <p className="text-sm text-muted-foreground">
          glTF 校验规范原文：KhronosGroup/glTF 5decc120；Copyright The Khronos
          Group Inc.
        </p>
      </div>
    </main>
  );
}
