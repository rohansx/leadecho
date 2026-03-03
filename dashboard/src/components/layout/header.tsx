import { Text } from "@/components/ui/text";
import { Moon, Sun, Bell, LogOut } from "lucide-react";
import { useTheme } from "@/providers/theme-provider";
import { useAuth } from "@/lib/auth";

export function Header() {
  const { theme, toggleTheme } = useTheme();
  const { user, logout } = useAuth();

  const initials = user?.name
    ? user.name
        .split(" ")
        .map((w) => w[0])
        .join("")
        .toUpperCase()
        .slice(0, 2)
    : "U";

  return (
    <header className="h-[var(--header-height)] border-b-2 border-border bg-card flex items-center justify-between px-6 shrink-0">
      <Text as="h4" className="text-foreground">
        Dashboard
      </Text>
      <div className="flex items-center gap-2">
        <button
          onClick={toggleTheme}
          className="w-9 h-9 rounded border-2 border-border bg-background shadow-xs hover:shadow-none transition-all flex items-center justify-center cursor-pointer"
        >
          {theme === "dark" ? (
            <Sun className="h-4 w-4" />
          ) : (
            <Moon className="h-4 w-4" />
          )}
        </button>
        <button className="w-9 h-9 rounded border-2 border-border bg-background shadow-xs hover:shadow-none transition-all flex items-center justify-center cursor-pointer">
          <Bell className="h-4 w-4" />
        </button>
        {user && (
          <>
            <div className="w-9 h-9 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center font-[family-name:var(--font-head)] text-primary-foreground text-xs">
              {initials}
            </div>
            <button
              onClick={logout}
              className="w-9 h-9 rounded border-2 border-border bg-background shadow-xs hover:shadow-none transition-all flex items-center justify-center cursor-pointer"
              title="Log out"
            >
              <LogOut className="h-4 w-4" />
            </button>
          </>
        )}
        {!user && (
          <div className="w-9 h-9 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center font-[family-name:var(--font-head)] text-primary-foreground text-xs">
            U
          </div>
        )}
      </div>
    </header>
  );
}
