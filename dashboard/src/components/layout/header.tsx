import { useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Moon, Sun, Bell, Menu } from "lucide-react";
import { useTheme } from "@/providers/theme-provider";
import { mentionQueueCounts } from "@/lib/api";
import { INBOX_QUEUES, QUERY_KEYS } from "@/lib/constants";
import { GlobalSearch } from "./global-search";

interface HeaderProps {
  /** Phones: opens the navigation drawer. */
  onMenu?: () => void;
}

export function Header({ onMenu }: HeaderProps) {
  const { theme, toggleTheme } = useTheme();
  const navigate = useNavigate();

  // Leads waiting for you (the Inbox "Leads" queue), not every unread post.
  const { data: queueCounts } = useQuery({
    queryKey: [QUERY_KEYS.mentionQueueCounts],
    queryFn: mentionQueueCounts,
    refetchInterval: 60_000,
  });
  const newCount = queueCounts?.queues.find((q) => q.queue === INBOX_QUEUES.ACTION_REQUIRED)?.count ?? 0;

  return (
    <header className="flex h-[var(--header-height)] shrink-0 items-center gap-4 border-b border-border bg-background/80 px-3 backdrop-blur md:px-5">
      <button
        type="button"
        onClick={onMenu}
        aria-label="Open menu"
        className="flex size-8 cursor-pointer items-center justify-center rounded-lg text-muted-foreground hover:bg-accent hover:text-foreground md:hidden"
      >
        <Menu className="h-5 w-5" />
      </button>
      <div className="flex min-w-0 flex-1 justify-center md:justify-start">
        <GlobalSearch />
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
          onClick={() => navigate({ to: "/inbox", search: { queue: INBOX_QUEUES.ACTION_REQUIRED } })}
          aria-label={newCount ? `${newCount} leads waiting` : "No leads waiting"}
          title="Leads waiting for you"
          className="relative flex size-7 cursor-pointer items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <Bell className="h-4 w-4" />
          {newCount > 0 && (
            <span className="absolute -top-0.5 -right-0.5 h-4 min-w-4 px-1 rounded-full bg-primary text-primary-foreground text-[10px] leading-none font-semibold tabular-nums flex items-center justify-center">
              {newCount > 99 ? "99+" : newCount}
            </span>
          )}
        </button>
      </div>
    </header>
  );
}
