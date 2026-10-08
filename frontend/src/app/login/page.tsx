import { AuthView } from "@/components/auth/auth-view";

export const metadata = { title: "登录" };
export default function Page() {
  return <AuthView mode="login" />;
}
