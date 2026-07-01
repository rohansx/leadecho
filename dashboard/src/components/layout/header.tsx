import { useState } from "react";
import { useNavigate, useRouterState } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Moon, Sun, Bell, LogOut, Search } from "lucide-react";
import { useTheme } from "@/providers/theme-provider";
import { useAuth } from "@/lib/auth";
import { mentionCounts } from "@/lib/api";

export function Header() {
  const { theme, toggleTheme } = useTheme();
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const [query, setQuery] = useState("");

  const { data: counts } = useQuery({
    queryKey: ["mentionCounts"],
    queryFn: mentionCounts,
  });
  const newCount = counts?.find((c) => c.status === "new")?.count ?? 0;

  const initials = user?.name
    ? user.name
        .split(" ")
        .map((w) => w[0])
        .join("")
        .toUpperCase()
        .slice(0, 2)
    : "U";

  const submitSearch = () => {
    if (!query.trim()) return;
    navigate({ to: "/inbox", search: { q: query.trim() } });
  };

  return (
    <header className="h-[var(--header-height)] border-b border-border bg-card flex items-center gap-4 px-6 shrink-0">
      <div className="relative flex-1 max-w-md">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && submitSearch()}
          placeholder="Search mentions…"
          className="w-full rounded-lg border border-border bg-background pl-9 pr-14 py-1.5 text-sm focus:outline-hidden focus:ring-2 focus:ring-ring/30 focus:border-ring placeholder:text-muted-foreground"
        />
        <span className="absolute right-2.5 top-1/2 -translate-y-1/2 text-[10px] font-[family-name:var(--font-mono)] text-muted-foreground border border-border rounded px-1.5 py-0.5">
          ⏎
        </span>
      </div>

      <div className="flex items-center gap-2 ml-auto">
        <button
          onClick={toggleTheme}
          aria-label="Toggle theme"
          className="w-9 h-9 rounded-lg border border-border bg-background hover:bg-accent transition-colors flex items-center justify-center cursor-pointer"
        >
          {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        </button>
        <button
          onClick={() => pathname !== "/inbox" && navigate({ to: "/inbox" })}
          aria-label="New mentions"
          className="relative w-9 h-9 rounded-lg border border-border bg-background hover:bg-accent transition-colors flex items-center justify-center cursor-pointer"
        >
          <Bell className="h-4 w-4" />
          {newCount > 0 && (
            <span className="absolute -top-1 -right-1 h-4 min-w-4 px-1 rounded-full bg-primary text-primary-foreground text-[10px] font-medium flex items-center justify-center">
              {newCount > 99 ? "99+" : newCount}
            </span>
          )}
        </button>
        {user ? (
          <>
            <div className="w-9 h-9 rounded-full bg-accent-soft text-primary-ink flex items-center justify-center font-[family-name:var(--font-head)] text-xs font-medium">
              {initials}
            </div>
            <button
              onClick={logout}
              className="w-9 h-9 rounded-lg border border-border bg-background hover:bg-accent transition-colors flex items-center justify-center cursor-pointer"
              title="Log out"
            >
              <LogOut className="h-4 w-4" />
            </button>
          </>
        ) : (
          <div className="w-9 h-9 rounded-full bg-accent-soft text-primary-ink flex items-center justify-center font-[family-name:var(--font-head)] text-xs font-medium">
            U
          </div>
        )}
      </div>
    </header>
  );
}
