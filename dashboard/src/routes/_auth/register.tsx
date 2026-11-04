import { createFileRoute } from "@tanstack/react-router";
import { AuthPanel } from "@/components/auth/auth-panel";

export const Route = createFileRoute("/_auth/register")({
  component: RegisterPage,
});

function RegisterPage() {
  return <AuthPanel mode="register" />;
}
