import { Link } from "@tanstack/react-router";
import { Moon, Sun } from "lucide-react";
import { useTheme } from "@/providers/theme-provider";
import { Logo } from "./logo";
import { GITHUB_URL } from "@/lib/constants";

const links = [
  { href: "#how", label: "How it works" },
  { href: "#pricing", label: "Pricing" },
  { href: "#self-host", label: "Self-host" },
  { href: "#faq", label: "FAQ" },
];

export function MarketingNav() {
  const { theme, toggleTheme } = useTheme();

  return (
    <header className="sticky top-0 z-40 border-b border-border bg-background/85 backdrop-blur">
      <div className="mx-auto flex h-16 max-w-[1200px] items-center gap-4 px-5 sm:px-10">
        <Logo />
        <nav className="mx-auto hidden items-center gap-7 md:flex">
          {links.map((l) => (
            <a key={l.href} href={l.href} className="text-[15px] text-foreground-soft transition-colors hover:text-foreground">
              {l.label}
            </a>
          ))}
        </nav>
        <div className="ml-auto flex items-center gap-2 md:ml-0">
          <a
            href={GITHUB_URL}
            target="_blank"
            rel="noreferrer"
            className="hidden h-9 items-center px-2 text-[15px] text-foreground-soft hover:text-foreground lg:inline-flex"
          >
            GitHub
          </a>
          <button
            onClick={toggleTheme}
            aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            className="flex h-9 w-9 cursor-pointer items-center justify-center rounded-full text-foreground-soft transition-colors hover:bg-accent hover:text-foreground"
          >
            {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </button>
          <Link to="/login" className="hidden h-9 items-center px-2 text-[15px] text-foreground-soft hover:text-foreground sm:inline-flex">
            Sign in
          </Link>
          <Link
            to="/register"
            className="inline-flex h-9 items-center rounded-full bg-foreground px-4 text-[14px] font-medium text-background transition-colors hover:bg-foreground/85"
          >
            Start free
          </Link>
        </div>
      </div>
    </header>
  );
}
