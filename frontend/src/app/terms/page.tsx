import packageManifest from "../../../public/legal/2026-10-15.1/manifest.json";
import { PublicPolicy } from "@/components/legal/public-policy";

export const metadata = { title: "服务条款" };

export default function Page() {
  return (
    <PublicPolicy
      document={packageManifest.documents[0]}
      version={packageManifest.version}
      filename="terms.md"
    />
  );
}
