import { ContentState } from "@/components/layout/content-state";
import { CircleHelp } from "lucide-react";
import {
  Dialog,
  DialogContent,
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
      <DialogContent aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>{feature}暂未开放</DialogTitle>
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
  description?: string;
}) {
  return (
    <ContentState icon={CircleHelp} title={title} description={description} />
  );
}
