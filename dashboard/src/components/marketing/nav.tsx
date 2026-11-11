import { useEffect, useState } from "react";
import { Link } from "@tanstack/react-router";
import { Moon, Sun } from "lucide-react";
import { useTheme } from "@/providers/theme-provider";
import { cn } from "@/lib/utils";
import { Logo } from "./logo";
import { GITHUB_URL } from "@/lib/constants";
import { container } from "./buttons";

const links = [
  { href: "#product", label: "Product" },
  { href: "#pricing", label: "Pricing" },
  { href: "#self-host", label: "Self-host" },
  { href: "#faq", label: "FAQ" },
];

export function MarketingNav() {
  const { theme, toggleTheme } = useTheme();
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const on = () => setScrolled(window.scrollY > 8);
    on();
    window.addEventListener("scroll", on, { passive: true });
    return () => window.removeEventListener("scroll", on);
  }, []);

  return (
    <header
      className={cn(
        "sticky top-0 z-40 border-b transition-[background-color,border-color] duration-300",
        scrolled ? "border-border bg-background/80 backdrop-blur-md" : "border-transparent",
      )}
    >
      <div className={cn(container, "flex h-16 items-center gap-6")}>
        <Logo />
        <nav className="hidden items-center gap-7 md:flex">
          {links.map((l) => (
            <a key={l.href} href={l.href} className="text-[15px] text-foreground-soft transition-colors hover:text-foreground">
              {l.label}
            </a>
          ))}
          <a href={GITHUB_URL} target="_blank" rel="noreferrer" className="text-[15px] text-foreground-soft transition-colors hover:text-foreground">
            GitHub
          </a>
        </nav>
        <div className="ml-auto flex items-center gap-1.5">
          <button
            onClick={toggleTheme}
            aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            className="flex h-9 w-9 cursor-pointer items-center justify-center rounded-full text-foreground-soft transition-colors hover:bg-accent hover:text-foreground"
          >
            {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </button>
          <Link to="/login" className="hidden h-9 items-center px-3 text-[15px] text-foreground-soft hover:text-foreground sm:inline-flex">
            Sign in
          </Link>
          <Link
            to="/register"
            className="inline-flex h-9 items-center rounded-full bg-primary-bg px-4 text-[14px] font-semibold text-primary-fg transition-opacity hover:opacity-90"
          >
            Start free
          </Link>
        </div>
      </div>
    </header>
  );
}
