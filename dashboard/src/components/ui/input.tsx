import { forwardRef } from "react";
import type { InputHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

type InputProps = InputHTMLAttributes<HTMLInputElement>;

const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, type = "text", ...props }, ref) => (
    <input
      type={type}
      className={cn(
        "px-3.5 py-2 w-full rounded-lg border border-border shadow-xs transition-shadow focus:outline-hidden focus:ring-2 focus:ring-ring/30 focus:border-ring font-[family-name:var(--font-sans)] bg-card text-foreground placeholder:text-muted-foreground",
        props["aria-invalid"] &&
          "border-destructive text-destructive focus:ring-destructive/30",
        className,
      )}
      ref={ref}
      {...props}
    />
  ),
);
Input.displayName = "Input";

export { Input };
