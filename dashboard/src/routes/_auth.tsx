import { createFileRoute, Outlet, Navigate } from "@tanstack/react-router";
import { useAuth } from "@/lib/auth";
import { Text } from "@/components/ui/text";

export const Route = createFileRoute("/_auth")({
  component: AuthLayout,
});

function AuthLayout() {
  const { user, loading } = useAuth();

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center bg-background">
        <Text as="p" className="text-muted-foreground">
          Loading...
        </Text>
      </div>
    );
  }

  if (user) {
    return <Navigate to="/inbox" />;
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background p-4">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <span className="font-[family-name:var(--font-head)] text-3xl">
            LeadEcho
          </span>
        </div>
        <Outlet />
      </div>
    </div>
  );
}
