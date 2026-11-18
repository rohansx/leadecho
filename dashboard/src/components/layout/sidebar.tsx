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
  Bell,
  Settings,
  Users,
  // enrich,
  Lightbulb,
  Binoculars,
} from "lucide-react";
import { ShieldCheck, Sparkles, X } from "lucide-react"; // chat (P2 #15), ops
import { opsMe } from "@/lib/ops-api";

const navGroups = [
  {
    label: "Work",
    items: [
      { to: "/inbox", label: "Inbox", icon: Inbox },
      { to: "/pipeline", label: "Pipeline", icon: GitBranch },
      { to: "/analytics", label: "Analytics", icon: BarChart3 },
      { to: "/demand", label: "Demand", icon: Lightbulb }, // what the market is asking for
      { to: "/competitors", label: "Competitors", icon: Binoculars }, // competitor watch
      { to: "/chat", label: "Ask LeadEcho", icon: Sparkles }, // chat (P2 #15)
    ],
  },
  {
    label: "Targeting",
    items: [
      { to: "/keywords", label: "Keywords", icon: Search },
      { to: "/profiles", label: "Profiles", icon: Target },
      { to: "/knowledge-base", label: "Knowledge Base", icon: BookOpen },
      { to: "/workflows", label: "Workflows", icon: Zap },
      { to: "/engagers", label: "LinkedIn engagers", icon: Users }, // enrich
    ],
  },
  {
    label: "Workspace",
    items: [
      { to: "/alerts", label: "Alerts", icon: Bell },
      { to: "/settings", label: "Settings", icon: Settings },
    ],
  },
] as const;

interface SidebarProps {
  /** Phones: the sidebar is a drawer, shown while open. */
  open?: boolean;
  onClose?: () => void;
}

export function Sidebar({ open = false, onClose }: SidebarProps) {
  const router = useRouterState();
  const pathname = router.location.pathname;

  const { data: tierCounts } = useQuery({
    queryKey: [QUERY_KEYS.mentionTierCounts],
    queryFn: mentionTierCounts,
  });
  const leadsReady = tierCounts?.find((c) => c.tier === MENTION_TIERS.LEADS_READY)?.count ?? 0;
  // ops: only the operator's allowlisted emails get a 200 here.
  const { data: isOps } = useQuery({ queryKey: ["ops", "me"], queryFn: opsMe, retry: false, staleTime: 5 * 60_000 });

  return (
    <>
      {/* Phones: a scrim behind the open drawer; tapping it closes the menu. */}
      {open && <div className="fixed inset-0 z-40 bg-black/40 md:hidden" onClick={onClose} aria-hidden="true" />}
    <aside
      className={cn(
        "flex w-[var(--sidebar-width)] shrink-0 flex-col border-r border-border bg-card",
        "fixed inset-y-0 left-0 z-50 transition-transform duration-200 md:static md:z-auto md:translate-x-0 md:bg-card/60",
        open ? "translate-x-0 shadow-lg" : "-translate-x-full",
      )}
      aria-label="Main navigation"
    >
      <div className="flex h-[var(--header-height)] min-w-0 shrink-0 items-center justify-between px-[18px]">
        <Logo to="/inbox" className="min-w-0 gap-2.5 text-base [&>svg]:size-5 [&>svg]:shrink-0 [&>svg]:overflow-visible" />
        <button type="button" onClick={onClose} aria-label="Close menu" className="flex size-7 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent md:hidden">
          <X className="h-4 w-4" />
        </button>
      </div>

      <nav className="flex-1 space-y-4 overflow-y-auto px-2.5 pb-4 pt-2">
        {isOps?.ops && (
          <Link
            to="/ops"
            className={cn(
              "flex h-9 md:h-7 items-center gap-2 rounded-md px-2 text-[13px]",
              pathname.startsWith("/ops") ? "bg-accent font-medium text-foreground" : "text-muted-foreground hover:bg-accent hover:text-foreground",
            )}
          >
            <ShieldCheck className="h-4 w-4 shrink-0" /> Operator console
          </Link>
        )}
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
                      "relative flex h-9 md:h-7 items-center gap-2 rounded-md px-2 text-[13px] transition-colors duration-150",
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
    </>
  );
}
