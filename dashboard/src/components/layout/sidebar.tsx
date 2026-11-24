import { Link, useRouterState } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { motion } from "motion/react";
import { cn } from "@/lib/utils";
import { mentionQueueCounts } from "@/lib/api";
import { INBOX_QUEUES, QUERY_KEYS } from "@/lib/constants";
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
import { ChevronRight, ShieldCheck, Sparkles, X } from "lucide-react"; // chat (P2 #15), ops
import { opsMe } from "@/lib/ops-api";
import { useState } from "react";
import { AccountMenu } from "./account-menu";

// Lead work first, then what steers it. Everything else sits under "More"
// (collapsed) until we decide what stays.
const navGroups = [
  {
    label: "Leads",
    items: [
      { to: "/inbox", label: "Inbox", icon: Inbox },
      { to: "/pipeline", label: "Pipeline", icon: GitBranch },
      { to: "/analytics", label: "Analytics", icon: BarChart3 },
    ],
  },
  {
    label: "Setup",
    items: [
      { to: "/keywords", label: "Keywords", icon: Search },
      { to: "/profiles", label: "Profiles", icon: Target },
      { to: "/knowledge-base", label: "Knowledge Base", icon: BookOpen },
      { to: "/settings", label: "Settings", icon: Settings },
    ],
  },
] as const;

const moreItems = [
  { to: "/demand", label: "Demand", icon: Lightbulb }, // what the market is asking for
  { to: "/competitors", label: "Competitors", icon: Binoculars }, // competitor watch
  { to: "/chat", label: "Ask LeadEcho", icon: Sparkles }, // chat (P2 #15)
  { to: "/workflows", label: "Workflows", icon: Zap },
  { to: "/engagers", label: "LinkedIn engagers", icon: Users }, // enrich
  { to: "/alerts", label: "Alerts", icon: Bell },
] as const;

type NavItem = (typeof navGroups)[number]["items"][number] | (typeof moreItems)[number];

interface SidebarProps {
  /** Phones: the sidebar is a drawer, shown while open. */
  open?: boolean;
  onClose?: () => void;
}

export function Sidebar({ open = false, onClose }: SidebarProps) {
  const router = useRouterState();
  const pathname = router.location.pathname;

  // Same number as the bell and the Inbox "Leads" tab: leads waiting for you.
  const { data: queueCounts } = useQuery({
    queryKey: [QUERY_KEYS.mentionQueueCounts],
    queryFn: mentionQueueCounts,
    refetchInterval: 60_000,
  });
  const leadsReady = queueCounts?.queues.find((q) => q.queue === INBOX_QUEUES.ACTION_REQUIRED)?.count ?? 0;
  // ops: only the operator's allowlisted emails get a 200 here.
  const { data: isOps } = useQuery({ queryKey: ["ops", "me"], queryFn: opsMe, retry: false, staleTime: 5 * 60_000 });
  const [moreOpen, setMoreOpen] = useState(false);
  // On one of its pages, "More" stays open so the current page is visible.
  const showMore = moreOpen || moreItems.some((i) => pathname.startsWith(i.to));

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
              {group.items.map((item) => <NavLink key={item.label} item={item} pathname={pathname} badge={item.to === "/inbox" ? leadsReady : 0} />)}
            </div>
          </div>
        ))}
        <div>
          <button
            type="button"
            onClick={() => setMoreOpen((v) => !v)}
            aria-expanded={showMore}
            className="flex w-full cursor-pointer items-center gap-1 px-2.5 pb-1.5 text-[11px] font-medium uppercase tracking-[0.08em] text-muted-foreground/80 hover:text-foreground"
          >
            More
            <ChevronRight className={cn("h-3 w-3 transition-transform", showMore && "rotate-90")} />
          </button>
          {showMore && (
            <div className="space-y-px">
              {moreItems.map((item) => <NavLink key={item.label} item={item} pathname={pathname} />)}
            </div>
          )}
        </div>
      </nav>
      <AccountMenu onNavigate={onClose} />
    </aside>
    </>
  );
}

function NavLink({ item, pathname, badge = 0 }: { item: NavItem; pathname: string; badge?: number }) {
  const isActive = pathname.startsWith(item.to);
  const Icon = item.icon;
  return (
    <Link
      to={item.to}
      className={cn(
        "relative flex h-9 md:h-7 items-center gap-2 rounded-md px-2 text-[13px] transition-colors duration-150",
        isActive ? "font-medium text-foreground" : "text-muted-foreground hover:bg-accent hover:text-foreground",
      )}
    >
      {isActive && (
        <motion.div
          layoutId="sidebar-active"
          className="absolute inset-0 rounded-md bg-accent shadow-[inset_0_0_0_1px_var(--border)]"
          transition={{ type: "spring", stiffness: 500, damping: 40 }}
        />
      )}
      <Icon className={cn("relative z-10 h-4 w-4 shrink-0", isActive ? "text-primary-ink" : "text-muted-foreground")} />
      <span className="relative z-10 flex-1">{item.label}</span>
      {badge > 0 && (
        <span
          aria-label={`${badge} leads ready`}
          className="relative z-10 min-w-5 rounded-md bg-accent-soft px-1.5 py-px text-center text-[11px] font-medium text-primary-ink tabular-nums"
        >
          {badge}
        </span>
      )}
    </Link>
  );
}
