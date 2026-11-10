import { Link } from "@tanstack/react-router";
import { Moon, Sun } from "lucide-react";
import { useTheme } from "@/providers/theme-provider";
import { Logo } from "./logo";
import { GITHUB_URL } from "@/lib/constants";

const links = [
  { href: "#how", label: "How it works" },
  { href: "#pricing", label: "Pricing" },
  { href: "#faq", label: "FAQ" },
  { href: GITHUB_URL, label: "GitHub", external: true },
];

export function MarketingNav() {
  const { theme, toggleTheme } = useTheme();

  return (
    <header className="sticky top-0 z-40 border-b border-border bg-background/90 backdrop-blur">
      <div className="mx-auto flex h-16 max-w-6xl items-center gap-8 px-4 sm:px-6">
        <Logo />
        <nav className="hidden items-center gap-6 md:flex">
          {links.map((l) => (
            <a
              key={l.href}
              href={l.href}
              {...(l.external ? { target: "_blank", rel: "noreferrer" } : {})}
              className="text-[15px] text-foreground-soft transition-colors hover:text-foreground"
            >
              {l.label}
            </a>
          ))}
        </nav>
        <div className="ml-auto flex items-center gap-4">
          <button
            onClick={toggleTheme}
            aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            className="flex h-9 w-9 cursor-pointer items-center justify-center rounded-lg text-foreground-soft transition-colors hover:bg-accent hover:text-foreground"
          >
            {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </button>
          <Link to="/login" className="hidden text-[15px] text-foreground-soft hover:text-foreground sm:inline">
            Sign in
          </Link>
          <Link
            to="/register"
            className="inline-flex h-9 items-center rounded-lg bg-foreground px-4 text-[15px] font-semibold text-background transition-colors hover:bg-foreground/85"
          >
            Start free
          </Link>
        </div>
      </div>
    </header>
  );
}
