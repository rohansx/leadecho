import {
  createFileRoute,
  Outlet,
  Navigate,
  useNavigate,
  useRouterState,
} from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { motion } from "motion/react";
import { FeedbackWidget } from "@/components/feedback-widget";
import { KeywordTopUp } from "@/components/keywords/keyword-top-up";
import { Sidebar } from "@/components/layout/sidebar";
import { Header } from "@/components/layout/header";
import { useAuth } from "@/lib/auth";
import { Text } from "@/components/ui/text";
import { getOnboardingStatus } from "@/lib/api";
import { cn } from "@/lib/utils";

export const Route = createFileRoute("/_dashboard")({
  component: DashboardLayout,
});

function DashboardLayout() {
  const { user, loading } = useAuth();
  const navigate = useNavigate();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  // Phones: the navigation drawer, closed again on every page change.
  const [navOpen, setNavOpen] = useState(false);
  const [navPath, setNavPath] = useState(pathname);
  if (navPath !== pathname) {
    setNavPath(pathname);
    setNavOpen(false);
  }

  const { data: onboarding } = useQuery({
    queryKey: ["onboarding"],
    queryFn: getOnboardingStatus,
    enabled: !!user,
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
      <div className="flex h-dvh items-center justify-center bg-background">
        <Text as="p" className="text-muted-foreground">
          Loading...
        </Text>
      </div>
    );
  }

  if (!user) {
    return <Navigate to="/login" replace />;
  }

  return (
    <div className="relative isolate flex h-dvh overflow-hidden bg-background">
      <div className="app-aura" aria-hidden="true" />
      <FeedbackWidget />
      <KeywordTopUp />
      <Sidebar open={navOpen} onClose={() => setNavOpen(false)} />
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <Header onMenu={() => setNavOpen(true)} />
        <main className="flex-1 overflow-auto">
          <motion.div
            key={pathname}
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.18, ease: "easeOut" }}
            className={
              // Inbox is a full-bleed split pane; every other page gets the reading container.
              pathname.startsWith("/inbox")
                ? "h-full"
                : cn(
                    "w-full px-4 py-4 md:px-6 md:py-6",
                    // Kanban needs every column visible; other pages read better in a container.
                    !pathname.startsWith("/pipeline") && "mx-auto max-w-[1200px]",
                  )
            }
          >
            <Outlet />
          </motion.div>
        </main>
      </div>
    </div>
  );
}
