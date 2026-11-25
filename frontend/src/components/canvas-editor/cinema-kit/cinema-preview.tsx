import Image from "next/image";
import assets from "./preview-assets.json";

type PreviewAsset = { image: string };
const previews: Record<string, PreviewAsset> = assets;

export function cinemaPreviewAsset(kind: string, name: string) {
  return previews[kind + ":" + name];
}

export function CinemaPreview({
  kind = "style",
  name = "复古对称美学",
  colors,
}: {
  kind?: "camera" | "palette" | "style" | "motion";
  name?: string;
  colors?: string[];
}) {
  const asset =
    cinemaPreviewAsset(kind, name) ??
    (kind === "camera" ? previews["camera:自动"] : undefined);
  if (!asset)
    return (
      <span
        className="cinema-preview cinema-preview-colors"
        aria-label={name}
        role="img"
      >
        {colors?.map((color, index) => (
          <i key={index} style={{ backgroundColor: color }} />
        ))}
      </span>
    );
  return (
    <div className="cinema-preview">
      <Image
        unoptimized
        src={asset.image}
        alt={name}
        width={640}
        height={360}
        sizes={
          kind === "palette" || kind === "motion"
            ? "(max-width: 640px) 45vw, 11rem"
            : "(max-width: 640px) 85vw, 36rem"
        }
      />
    </div>
  );
}
