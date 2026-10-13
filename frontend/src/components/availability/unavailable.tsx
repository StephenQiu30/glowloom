import { CircleHelp } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
export function UnavailableDialog({
  feature,
  onClose,
}: {
  feature: string | null;
  onClose: () => void;
}) {
  return (
    <Dialog
      open={feature !== null}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{feature}暂未开放</DialogTitle>
          <DialogDescription>
            此功能还未接入服务。你可以继续使用画布整理图片、视频草稿，并保存创作内容。
          </DialogDescription>
        </DialogHeader>
      </DialogContent>
    </Dialog>
  );
}
export function UnavailableState({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <div className="empty-state">
      <CircleHelp className="size-9" strokeWidth={1.3} />
      <h2>{title}</h2>
      <p>{description}</p>
    </div>
  );
}
