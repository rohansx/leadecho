import { useEffect, useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import { ChevronsUpDown, LogOut, Moon, Settings, Sun, UserRound } from "lucide-react";
import { useAuth } from "@/lib/auth";
import { useTheme } from "@/providers/theme-provider";
import { cn } from "@/lib/utils";

function initialsOf(name: string | undefined): string {
  const parts = (name ?? "").trim().split(/\s+/).filter(Boolean);
  return parts.length ? parts.map((w) => w[0]).join("").toUpperCase().slice(0, 2) : "U";
}

const itemClass =
  "flex h-8 w-full cursor-pointer items-center gap-2 rounded-md px-2 text-[13px] text-foreground-soft hover:bg-accent hover:text-foreground";

/** Bottom of the sidebar: who is signed in, and the account actions. */
export function AccountMenu({ onNavigate }: { onNavigate?: () => void }) {
  const { user, logout } = useAuth();
  const { theme, toggleTheme } = useTheme();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent ? e.key === "Escape" : !ref.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", close);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", close);
    };
  }, [open]);

  const go = () => {
    setOpen(false);
    onNavigate?.();
  };

  return (
    <div ref={ref} className="relative border-t border-border p-2.5">
      {open && (
        <div role="menu" className="absolute inset-x-2.5 bottom-full mb-1 rounded-lg border border-border bg-card p-1 shadow-lg">
          <Link to="/settings" search={{ tab: "account" }} onClick={go} role="menuitem" className={itemClass}>
            <UserRound className="h-4 w-4" /> Account
          </Link>
          <Link to="/settings" onClick={go} role="menuitem" className={itemClass}>
            <Settings className="h-4 w-4" /> Settings
          </Link>
          <button type="button" role="menuitem" onClick={toggleTheme} className={itemClass}>
            {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            {theme === "dark" ? "Light mode" : "Dark mode"}
          </button>
          <div className="my-1 h-px bg-border" />
          <button type="button" role="menuitem" onClick={() => void logout()} className={itemClass}>
            <LogOut className="h-4 w-4" /> Log out
          </button>
        </div>
      )}
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="menu"
        aria-expanded={open}
        className={cn("flex w-full cursor-pointer items-center gap-2.5 rounded-md p-1.5 text-left hover:bg-accent", open && "bg-accent")}
      >
        <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-accent-soft text-[11px] font-medium text-primary-ink">
          {initialsOf(user?.name)}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13px] font-medium">{user?.name || "Account"}</span>
          <span className="block truncate text-[11px] text-muted-foreground">{user?.email}</span>
        </span>
        <ChevronsUpDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      </button>
    </div>
  );
}
