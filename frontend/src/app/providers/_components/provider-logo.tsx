import Image from "next/image";
import { Globe } from "lucide-react";

const logos: Record<string, string> = {
  openrouter: "/brand/providers/openrouter.svg",
  volcengine_ark: "/brand/providers/volcengine.svg",
  volcengine_speech: "/brand/providers/doubao.svg",
  byteplus: "/brand/providers/byteplus.png",
  deepseek: "/brand/providers/deepseek.svg",
};

export function ProviderLogo({ adapterKey }: { adapterKey?: string }) {
  const source =
    adapterKey && Object.hasOwn(logos, adapterKey)
      ? logos[adapterKey]
      : undefined;
  if (!source) return <Globe aria-hidden="true" className="size-5 shrink-0" />;

  if (adapterKey === "openrouter") {
    return (
      <span
        aria-hidden="true"
        className="size-5 shrink-0 bg-foreground"
        style={{
          maskImage: "url('/brand/providers/openrouter.svg')",
          maskSize: "contain",
          maskPosition: "center",
          maskRepeat: "no-repeat",
        }}
      />
    );
  }

  return (
    <Image
      src={source}
      alt=""
      width={20}
      height={20}
      unoptimized
      className="size-5 shrink-0 object-contain"
    />
  );
}
