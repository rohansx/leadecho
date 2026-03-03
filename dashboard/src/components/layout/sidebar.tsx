import { Link, useRouterState } from "@tanstack/react-router";
import { cn } from "@/lib/utils";
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
  Link2,
} from "lucide-react";

const navItems = [
  { to: "/inbox", label: "Inbox", icon: Inbox },
  { to: "/pipeline", label: "Pipeline", icon: GitBranch },
  { to: "/analytics", label: "Analytics", icon: BarChart3 },
  { to: "/analytics", label: "Tracking", icon: Link2 },
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

  return (
    <aside className="w-[var(--sidebar-width)] h-screen border-r-2 border-border bg-card flex flex-col shrink-0">
      {/* Logo */}
      <div className="h-[var(--header-height)] flex items-center px-4 border-b-2 border-border">
        <Link to="/" className="flex items-center gap-2">
          <div className="w-8 h-8 bg-primary border-2 border-border shadow-xs rounded flex items-center justify-center font-[family-name:var(--font-head)] text-primary-foreground text-sm font-bold">
            LE
          </div>
          <span className="font-[family-name:var(--font-head)] text-xl">
            LeadEcho
          </span>
        </Link>
      </div>

      {/* Navigation */}
      <nav className="flex-1 p-3 space-y-1 overflow-y-auto">
        {navItems.map((item) => {
          const isActive = pathname.startsWith(item.to);
          const Icon = item.icon;
          return (
            <Link
              key={item.to}
              to={item.to}
              className={cn(
                "flex items-center gap-3 px-3 py-2 rounded font-[family-name:var(--font-sans)] text-sm transition-all",
                "border-2 border-transparent",
                isActive
                  ? "bg-primary text-primary-foreground border-border shadow-sm font-medium"
                  : "hover:bg-accent hover:border-border hover:shadow-xs text-foreground",
              )}
            >
              <Icon className="h-4 w-4 shrink-0" />
              {item.label}
            </Link>
          );
        })}
      </nav>
    </aside>
  );
}
