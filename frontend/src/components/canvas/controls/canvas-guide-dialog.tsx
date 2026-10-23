import type { ReactNode } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Kbd, KbdGroup } from "@/components/ui/kbd";

const guides = {
  shortcuts: {
    title: "快捷键",
    description: "使用键盘选择、移动和编辑画布。",
  },
  tutorial: {
    title: "画布教程",
    description: "从节点开始，将想法连接成作品。",
  },
};
const shortcutGroups = [
  {
    title: "移动与缩放",
    items: [
      ["选择 / 抓手", ["V", "H"]],
      ["临时抓手", ["Space", "拖动"]],
      ["平移画布", ["滚轮"]],
      ["水平平移", ["Shift", "滚轮"]],
      ["缩放画布", ["Ctrl / ⌘", "滚轮"]],
      ["放大 / 缩小", ["Ctrl / ⌘", "+ / −"]],
      ["适合屏幕", ["Ctrl / ⌘", "0"]],
    ],
  },
  {
    title: "选择与整理",
    items: [
      ["框选节点", ["V", "拖动空白处"]],
      ["增减选择", ["Shift", "点击"]],
      ["全选节点", ["Ctrl / ⌘", "A"]],
      ["复制 / 粘贴", ["Ctrl / ⌘", "C / V"]],
      ["组合 / 解组", ["Ctrl / ⌘", "G / Shift G"]],
      ["撤销 / 重做", ["Ctrl / ⌘", "Z / Shift Z"]],
      ["删除选中", ["Delete / Backspace"]],
      ["添加节点（画布焦点）", ["Tab"]],
    ],
  },
  {
    title: "输入与弹层",
    items: [
      ["提示词换行", ["Enter / Shift Enter"]],
      ["生成（提示词焦点）", ["Ctrl / ⌘", "Enter"]],
      ["引用素材", ["@"]],
      ["选择引用", ["↑ / ↓", "Enter"]],
      ["关闭当前层 / 取消选择", ["Esc"]],
    ],
  },
] as const;

export function CanvasGuideDialog({
  kind,
  children,
}: {
  kind: keyof typeof guides;
  children: ReactNode;
}) {
  const guide = guides[kind];
  return (
    <Dialog>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent
        className={
          kind === "shortcuts"
            ? "top-auto bottom-20 dialog-canvas-shortcuts translate-y-0 overflow-y-auto"
            : "dialog-canvas max-h-dialog overflow-y-auto"
        }
      >
        <DialogHeader>
          <DialogTitle>{guide.title}</DialogTitle>
          <DialogDescription>{guide.description}</DialogDescription>
        </DialogHeader>
        {kind === "shortcuts" ? (
          <div className="grid gap-6 md:grid-cols-3">
            {shortcutGroups.map((group) => (
              <section key={group.title} className="flex flex-col gap-3">
                <h3 className="font-medium">{group.title}</h3>
                {group.items.map(([label, keys]) => (
                  <div
                    key={label}
                    className="flex flex-wrap items-center justify-between gap-2 text-xs"
                  >
                    <span className="text-muted-foreground">{label}</span>
                    <KbdGroup>
                      {keys.map((key) => (
                        <Kbd key={key}>{key}</Kbd>
                      ))}
                    </KbdGroup>
                  </div>
                ))}
              </section>
            ))}
            <p className="border-t pt-3 text-xs text-muted-foreground md:col-span-3">
              输入文字时，选择、删除与撤销只作用于文字。Esc
              每次关闭最上层；弹层打开时，画布快捷键暂停。
            </p>
          </div>
        ) : (
          <ol className="flex list-decimal flex-col gap-4 pl-5">
            <li>点击添加节点，选择图片、视频、音频或剧本。</li>
            <li>选中节点，在下方填写提示词并选择模型与参数。</li>
            <li>通过「参考」选择已有节点，将素材连接到当前创作。</li>
            <li>
              用抓手或滚轮平移，按住 Ctrl / ⌘
              滚轮缩放，点击适合屏幕查看全部节点。
            </li>
            <li>使用节点工具查看结果、编辑内容和整理分组。</li>
          </ol>
        )}
      </DialogContent>
    </Dialog>
  );
}
