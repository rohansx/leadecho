import { Link, useRouterState } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { motion } from "motion/react";
import { cn } from "@/lib/utils";
import { mentionTierCounts } from "@/lib/api";
import { Logo } from "@/components/marketing/logo";
import {
  Inbox,
  GitBranch,
  BarChart3,
  BookOpen,
  Zap,
  Search,
  Target,
  Globe,
  Bell,
  Settings,
} from "lucide-react";

const navItems = [
  { to: "/inbox", label: "Inbox", icon: Inbox },
  { to: "/pipeline", label: "Pipeline", icon: GitBranch },
  { to: "/analytics", label: "Analytics", icon: BarChart3 },
  { to: "/knowledge-base", label: "Knowledge Base", icon: BookOpen },
  { to: "/workflows", label: "Workflows", icon: Zap },
  { to: "/keywords", label: "Keywords", icon: Search },
  { to: "/profiles", label: "Profiles", icon: Target },
  { to: "/browser-sessions", label: "Browser", icon: Globe },
  { to: "/alerts", label: "Alerts", icon: Bell },
  { to: "/settings", label: "Settings", icon: Settings },
] as const;

export function Sidebar() {
  const router = useRouterState();
  const pathname = router.location.pathname;

  const { data: tierCounts } = useQuery({
    queryKey: ["mentionTierCounts"],
    queryFn: mentionTierCounts,
  });
  const leadsReady = tierCounts?.find((c) => c.tier === "leads_ready")?.count ?? 0;

  return (
    <aside className="w-[var(--sidebar-width)] h-screen border-r border-border bg-card flex flex-col shrink-0">
      <div className="h-[var(--header-height)] flex items-center px-5 border-b border-border">
        <Logo />
      </div>

      <nav className="flex-1 p-3 space-y-0.5 overflow-y-auto">
        {navItems.map((item) => {
          const isActive = pathname.startsWith(item.to);
          const Icon = item.icon;
          return (
            <Link
              key={item.label}
              to={item.to}
              className={cn(
                "relative flex items-center gap-3 px-3 py-2 rounded-lg font-[family-name:var(--font-sans)] text-sm transition-colors",
                isActive
                  ? "text-primary-foreground font-medium"
                  : "text-foreground-soft hover:bg-accent hover:text-foreground",
              )}
            >
              {isActive && (
                <motion.div
                  layoutId="sidebar-active"
                  className="absolute inset-0 bg-primary rounded-lg"
                  transition={{ type: "spring", stiffness: 400, damping: 34 }}
                />
              )}
              <Icon className="relative z-10 h-4 w-4 shrink-0" />
              <span className="relative z-10 flex-1">{item.label}</span>
              {item.to === "/inbox" && leadsReady > 0 && (
                <span
                  className={cn(
                    "relative z-10 text-[11px] font-medium rounded-full px-1.5 py-0.5 min-w-[1.25rem] text-center",
                    isActive ? "bg-primary-foreground/20" : "bg-accent-soft text-primary-ink",
                  )}
                >
                  {leadsReady}
                </span>
              )}
            </Link>
          );
        })}
      </nav>
    </aside>
  );
}
