import { useAppFeedback } from "@/components/canvas/runtime/components/feedback/app-feedback-provider";

import copy from "copy-to-clipboard";

export function useCopyText() {
  const { message } = useAppFeedback();

  return (value: string, successText = "已复制") => {
    copy(value);
    message.success(successText);
  };
}
