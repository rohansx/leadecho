import { Link, useRouterState } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { motion } from "motion/react";
import { cn } from "@/lib/utils";
import { mentionTierCounts } from "@/lib/api";
import { MENTION_TIERS, QUERY_KEYS } from "@/lib/constants";
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
    queryKey: [QUERY_KEYS.mentionTierCounts],
    queryFn: mentionTierCounts,
  });
  const leadsReady = tierCounts?.find((c) => c.tier === MENTION_TIERS.LEADS_READY)?.count ?? 0;

  return (
    <aside className="glass flex w-[var(--sidebar-width)] shrink-0 flex-col overflow-hidden rounded-2xl">
      <div className="flex h-[var(--header-height)] items-center border-b border-border/40 px-5">
        <Logo />
      </div>

      <nav className="flex-1 space-y-0.5 overflow-y-auto p-3">
        {navItems.map((item) => {
          const isActive = pathname.startsWith(item.to);
          const Icon = item.icon;
          return (
            <Link
              key={item.label}
              to={item.to}
              className={cn(
                "relative flex items-center gap-3 rounded-xl px-3 py-2 font-[family-name:var(--font-sans)] text-sm transition-colors duration-150",
                isActive
                  ? "font-medium text-primary-foreground"
                  : "text-foreground-soft hover:bg-accent/70 hover:text-foreground",
              )}
            >
              {isActive && (
                <motion.div
                  layoutId="sidebar-active"
                  className="absolute inset-0 rounded-xl bg-primary shadow-sm"
                  transition={{ type: "spring", stiffness: 400, damping: 34 }}
                />
              )}
              <Icon className="relative z-10 h-4 w-4 shrink-0" />
              <span className="relative z-10 flex-1">{item.label}</span>
              {item.to === "/inbox" && leadsReady > 0 && (
                <span
                  aria-label={`${leadsReady} leads ready`}
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
