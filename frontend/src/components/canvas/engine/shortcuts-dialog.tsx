"use client";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

const shortcuts = [
  ["选择 / 移动画布", "V / H"],
  ["保存", "⌘ / Ctrl + S"],
  ["撤销 / 重做", "⌘ / Ctrl + Z / Shift + Z"],
  ["复制 / 粘贴节点", "⌘ / Ctrl + C / V"],
  ["全选", "⌘ / Ctrl + A"],
  ["分组 / 解组", "⌘ / Ctrl + G / Shift + G"],
  ["搜索节点", "⌘ / Ctrl + F"],
  ["100% 缩放", "⌘ / Ctrl + 1"],
  ["适应全画布", "⌘ / Ctrl + 2 / 0"],
  ["适应选择", "⌘ / Ctrl + 3 / F"],
  ["放大 / 缩小", "⌘ / Ctrl + + / −"],
  ["自动布局", "Alt + Shift + F"],
  ["移动节点 / 快移", "方向键 / Shift + 方向键"],
  ["删除选择", "Delete / Backspace"],
  ["取消选择", "Esc"],
  ["切换专注模式", "画布焦点下 Tab / ⌘ / Ctrl + Shift + F"],
  ["快捷键帮助", "?"],
] as const;

export function CanvasShortcutsDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[80dvh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>画布快捷键</DialogTitle>
          <DialogDescription>
            输入框和弹窗保留原生操作；选中文字时复制文字。按钮上的 Tab
            继续导航。
          </DialogDescription>
        </DialogHeader>
        <dl className="grid gap-3">
          {shortcuts.map(([action, keys]) => (
            <div
              key={action}
              className="flex items-baseline justify-between gap-4"
            >
              <dt>{action}</dt>
              <dd className="text-right text-xs text-muted-foreground">
                <kbd>{keys}</kbd>
              </dd>
            </div>
          ))}
        </dl>
      </DialogContent>
    </Dialog>
  );
}
