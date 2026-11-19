import packageManifest from "../../../public/legal/2026-10-15.1/manifest.json";
import { PublicPolicy } from "@/components/legal/public-policy";

export const metadata = { title: "隐私政策" };

export default function Page() {
  return (
    <PublicPolicy
      document={packageManifest.documents[1]}
      version={packageManifest.version}
      filename="privacy.md"
    />
  );
}
