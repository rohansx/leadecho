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
  CalendarClock, // automations (P2 #17)
} from "lucide-react";

const navGroups = [
  {
    label: "Work",
    items: [
      { to: "/inbox", label: "Inbox", icon: Inbox },
      { to: "/pipeline", label: "Pipeline", icon: GitBranch },
      { to: "/analytics", label: "Analytics", icon: BarChart3 },
    ],
  },
  {
    label: "Targeting",
    items: [
      { to: "/keywords", label: "Keywords", icon: Search },
      { to: "/profiles", label: "Profiles", icon: Target },
      { to: "/knowledge-base", label: "Knowledge Base", icon: BookOpen },
      { to: "/workflows", label: "Workflows", icon: Zap },
      { to: "/automations", label: "Automations", icon: CalendarClock }, // automations (P2 #17)
    ],
  },
  {
    label: "Workspace",
    items: [
      { to: "/browser-sessions", label: "Browser", icon: Globe },
      { to: "/alerts", label: "Alerts", icon: Bell },
      { to: "/settings", label: "Settings", icon: Settings },
    ],
  },
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
    <aside className="flex w-[var(--sidebar-width)] shrink-0 flex-col border-r border-border bg-card/60">
      <div className="flex h-[var(--header-height)] items-center px-5">
        <Logo to="/inbox" />
      </div>

      <nav className="flex-1 space-y-5 overflow-y-auto px-3 pb-4 pt-2">
        {navGroups.map((group) => (
          <div key={group.label}>
            <p className="px-2.5 pb-1.5 text-[11px] font-medium uppercase tracking-[0.08em] text-muted-foreground/80">
              {group.label}
            </p>
            <div className="space-y-px">
              {group.items.map((item) => {
                const isActive = pathname.startsWith(item.to);
                const Icon = item.icon;
                return (
                  <Link
                    key={item.label}
                    to={item.to}
                    className={cn(
                      "relative flex h-8 items-center gap-2.5 rounded-md px-2.5 text-[13px] transition-colors duration-150",
                      isActive
                        ? "font-medium text-foreground"
                        : "text-muted-foreground hover:bg-accent hover:text-foreground",
                    )}
                  >
                    {isActive && (
                      <motion.div
                        layoutId="sidebar-active"
                        className="absolute inset-0 rounded-md bg-accent shadow-[inset_0_0_0_1px_var(--border)]"
                        transition={{ type: "spring", stiffness: 500, damping: 40 }}
                      />
                    )}
                    <Icon
                      className={cn(
                        "relative z-10 h-4 w-4 shrink-0",
                        isActive ? "text-primary-ink" : "text-muted-foreground",
                      )}
                    />
                    <span className="relative z-10 flex-1">{item.label}</span>
                    {item.to === "/inbox" && leadsReady > 0 && (
                      <span
                        aria-label={`${leadsReady} leads ready`}
                        className="relative z-10 min-w-5 rounded-md bg-accent-soft px-1.5 py-px text-center text-[11px] font-medium text-primary-ink tabular-nums"
                      >
                        {leadsReady}
                      </span>
                    )}
                  </Link>
                );
              })}
            </div>
          </div>
        ))}
      </nav>
    </aside>
  );
}
