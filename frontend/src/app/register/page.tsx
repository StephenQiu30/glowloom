import { AuthView } from "@/components/auth/auth-view";

export const metadata = { title: "注册" };
export default function Page() {
  return <AuthView mode="register" />;
}
