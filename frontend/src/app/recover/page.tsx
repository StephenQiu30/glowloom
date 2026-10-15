import { AuthView } from "@/components/auth/auth-view";
export const metadata = { title: "找回密码" };
export default function Page() {
  return <AuthView mode="recover" />;
}
