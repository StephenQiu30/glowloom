export function readImageMeta(source: string) {
  return new Promise<{ width: number; height: number; mimeType: string }>(
    (resolve, reject) => {
      const image = new Image();
      const finish = (error?: Error) => {
        clearTimeout(timer);
        image.onload = null;
        image.onerror = null;
        if (error) reject(error);
        else
          resolve({
            width: image.naturalWidth || image.width,
            height: image.naturalHeight || image.height,
            mimeType: source.match(/^data:([^;]+)/)?.[1] ?? "image/png",
          });
      };
      const timer = setTimeout(
        () => finish(new Error("图片读取超时，请重试")),
        15000,
      );
      image.crossOrigin = "anonymous";
      image.onload = () => finish();
      image.onerror = () => finish(new Error("图片无法读取，请重新获取素材"));
      image.src = source;
    },
  );
}
