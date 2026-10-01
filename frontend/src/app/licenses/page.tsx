import { WorkspaceShell } from "@/components/workbench/workspace-shell";

export default function LicensesPage() {
  return (
    <WorkspaceShell>
      <div className="max-w-2xl space-y-6">
        <h1 className="text-2xl font-medium">第三方许可</h1>
        <p className="leading-7 text-muted-foreground">
          Lanverse 的无限画布包含基于 BeefTV 与 Infinite Canvas 的 MIT
          许可代码，时间线算法同时保留 Lingji Cut 的 Apache-2.0 声明。
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
        </ul>
        <p className="text-sm text-muted-foreground">
          来源版本：BeefTV 0d9e9f48 画布核心；完整能力迁移参考 1ae25027。© 2026
          @beefnoode、BeefTV contributors、basketikun、ddcat、yoqu。
        </p>
        <p className="text-sm text-muted-foreground">
          glTF 校验规范原文：KhronosGroup/glTF 5decc120；Copyright The Khronos
          Group Inc.
        </p>
      </div>
    </WorkspaceShell>
  );
}
