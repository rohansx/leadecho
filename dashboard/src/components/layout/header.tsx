import { useState } from "react";
import { useNavigate, useRouterState } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Moon, Sun, Bell, LogOut, Search } from "lucide-react";
import { useTheme } from "@/providers/theme-provider";
import { useAuth } from "@/lib/auth";
import { mentionCounts } from "@/lib/api";
import { QUERY_KEYS } from "@/lib/constants";

export function Header() {
  const { theme, toggleTheme } = useTheme();
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const [query, setQuery] = useState("");

  const { data: counts } = useQuery({
    queryKey: [QUERY_KEYS.mentionCounts],
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
    <header className="flex h-[var(--header-height)] shrink-0 items-center gap-4 border-b border-border bg-background/80 px-5 backdrop-blur">
      <div className="relative max-w-sm flex-1">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && submitSearch()}
          placeholder="Search mentions…"
          className="h-7 w-full rounded-lg border border-border bg-card pl-9 pr-10 text-[13px] placeholder:text-muted-foreground focus:border-ring focus:outline-hidden focus:ring-3 focus:ring-ring/20"
        />
        <span className="absolute right-2.5 top-1/2 -translate-y-1/2 text-[10px] font-[family-name:var(--font-mono)] text-muted-foreground border border-border rounded px-1.5 py-0.5">
          ⏎
        </span>
      </div>

      <div className="ml-auto flex items-center gap-1">
        <button
          type="button"
          onClick={toggleTheme}
          aria-label="Toggle theme"
          className="flex size-7 cursor-pointer items-center justify-center rounded-lg text-muted-foreground transition-colors duration-150 hover:bg-accent hover:text-foreground"
        >
          {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        </button>
        <button
          type="button"
          onClick={() => pathname !== "/inbox" && navigate({ to: "/inbox" })}
          aria-label="New mentions"
          className="relative flex size-7 cursor-pointer items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <Bell className="h-4 w-4" />
          {newCount > 0 && (
            <span className="absolute -top-0.5 -right-0.5 h-4 min-w-4 px-1 rounded-full bg-primary text-primary-foreground text-[10px] font-semibold tabular-nums ring-2 ring-background flex items-center justify-center">
              {newCount > 99 ? "99+" : newCount}
            </span>
          )}
        </button>
        {user ? (
          <>
            <div className="ml-1 size-7 rounded-full bg-accent-soft text-primary-ink flex items-center justify-center text-[11px] font-semibold">
              {initials}
            </div>
            <button
              type="button"
              onClick={logout}
              className="flex size-7 cursor-pointer items-center justify-center rounded-lg text-muted-foreground transition-colors duration-150 hover:bg-accent hover:text-foreground"
              title="Log out"
            >
              <LogOut className="h-4 w-4" />
            </button>
          </>
        ) : (
          <div className="ml-1 size-7 rounded-full bg-accent-soft text-primary-ink flex items-center justify-center text-[11px] font-semibold">
            U
          </div>
        )}
      </div>
    </header>
  );
}
