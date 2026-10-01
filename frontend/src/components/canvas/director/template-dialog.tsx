"use client";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { DirectorScene } from "./model";
import {
  createDirectorSceneFromTemplate,
  DIRECTOR_TEMPLATES,
} from "./templates";

export function DirectorTemplateDialog({
  disabled,
  onClose,
  onSelect,
}: {
  disabled: boolean;
  onClose: () => void;
  onSelect: (scene: DirectorScene) => void;
}) {
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>选择镜头模板</DialogTitle>
          <DialogDescription>
            选择开局的演员、道具和机位；随后在导演台继续编辑。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-2 sm:grid-cols-2">
          {DIRECTOR_TEMPLATES.map((template) => (
            <Button
              key={template.id}
              variant="outline"
              className="h-auto min-h-28 items-start justify-start p-4 text-left whitespace-normal"
              disabled={disabled}
              onClick={() =>
                onSelect(
                  createDirectorSceneFromTemplate(template.id, template.name),
                )
              }
            >
              <span className="flex flex-col gap-1">
                <span className="font-medium">{template.name}</span>
                <span className="text-xs text-muted-foreground">
                  {template.summary}
                </span>
                <span className="text-xs text-muted-foreground">
                  {template.description}
                </span>
              </span>
            </Button>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
