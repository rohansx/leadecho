import { createFileRoute, Outlet, Navigate, useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { Sidebar } from "@/components/layout/sidebar";
import { Header } from "@/components/layout/header";
import { useAuth } from "@/lib/auth";
import { Text } from "@/components/ui/text";
import { getOnboardingStatus } from "@/lib/api";

export const Route = createFileRoute("/_dashboard")({
  component: DashboardLayout,
});

function DashboardLayout() {
  const { user, loading } = useAuth();
  const navigate = useNavigate();

  const { data: onboarding } = useQuery({
    queryKey: ["onboarding"],
    queryFn: getOnboardingStatus,
    enabled: !!user || window.location.hostname === "localhost",
    retry: false,
  });

  useEffect(() => {
    if (onboarding && !onboarding.completed) {
      const path = window.location.pathname;
      if (!path.includes("/onboarding")) {
        navigate({ to: "/onboarding" });
      }
    }
  }, [onboarding, navigate]);

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center bg-background">
        <Text as="p" className="text-muted-foreground">
          Loading...
        </Text>
      </div>
    );
  }

  // If Google OAuth is configured and user is not authenticated, redirect to login
  // In dev mode (no Google OAuth configured), /auth/me returns 404 and user is null,
  // so we allow access without auth for development
  if (!user && window.location.hostname !== "localhost") {
    return <Navigate to="/login" />;
  }

  return (
    <div className="flex h-screen overflow-hidden">
      <Sidebar />
      <div className="flex-1 flex flex-col overflow-hidden">
        <Header />
        <main className="flex-1 overflow-auto p-6 bg-background">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
