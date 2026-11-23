import Image from "next/image";

/** Fixed artwork shared by the workbench, galleries and studio templates. */
export function ReferenceCover({ image }: { image: string }) {
  return (
    <Image
      src={`/design/reference/covers/${image}.webp`}
      alt=""
      fill
      unoptimized
      loading="eager"
      className="object-cover"
    />
  );
}
