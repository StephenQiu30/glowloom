import { toast } from "sonner";

export const demoNotice = (message: string): void => {
  toast.success(`${message}（本地演示）`);
};
