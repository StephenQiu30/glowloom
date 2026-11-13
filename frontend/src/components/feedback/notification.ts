"use client";

import { toast as sonner, type ExternalToast } from "sonner";

function options(data: ExternalToast | undefined, duration = 4000) {
  return {
    ...data,
    duration:
      data?.action || data?.cancel ? Infinity : (data?.duration ?? duration),
  };
}

export const toast = Object.assign(
  (...[message, data]: Parameters<typeof sonner>) =>
    sonner(message, options(data)),
  {
    ...sonner,
    dismiss: (id?: number | string) => sonner.dismiss(id),
    success: (...[message, data]: Parameters<typeof sonner.success>) =>
      sonner.success(message, options(data)),
    info: (...[message, data]: Parameters<typeof sonner.info>) =>
      sonner.info(message, options(data)),
    warning: (...[message, data]: Parameters<typeof sonner.warning>) =>
      sonner.warning(message, options(data, 6000)),
    error: (...[message, data]: Parameters<typeof sonner.error>) =>
      sonner.error(message, options(data, 6000)),
    message: (...[message, data]: Parameters<typeof sonner.message>) =>
      sonner.message(message, options(data)),
    loading: (...[message, data]: Parameters<typeof sonner.loading>) =>
      sonner.loading(message, options(data, Infinity)),
  },
);
