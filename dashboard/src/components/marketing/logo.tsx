import { Link } from "@tanstack/react-router";
import { cn } from "@/lib/utils";

export function LogoMark({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      className={cn("text-primary-ink", className)}
      aria-hidden="true"
    >
      <circle cx="6" cy="12" r="2" fill="currentColor" stroke="none" />
      <path d="M10 7 a 7 7 0 0 1 0 10" />
      <path d="M14 4 a 11 11 0 0 1 0 16" />
      <path d="M18 1 a 15 15 0 0 1 0 22" strokeOpacity="0.5" />
    </svg>
  );
}

export function Logo({ className }: { className?: string }) {
  return (
    <Link
      to="/"
      aria-label="LeadEcho home"
      className={cn(
        "flex items-center gap-2 font-[family-name:var(--font-head)] text-lg",
        className,
      )}
    >
      <LogoMark className="h-6 w-6" />
      <span>
        <b className="font-semibold">Lead</b>
        <i className="font-[family-name:var(--font-serif)] italic text-primary-ink">Echo</i>
      </span>
    </Link>
  );
}
