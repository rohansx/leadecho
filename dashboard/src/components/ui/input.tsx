import { forwardRef } from "react";
import type { InputHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

type InputProps = InputHTMLAttributes<HTMLInputElement>;

const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, type = "text", ...props }, ref) => (
    <input
      type={type}
      className={cn(
        "h-9 px-3 text-sm w-full rounded-lg border border-input shadow-xs transition-shadow focus:outline-hidden focus:ring-3 focus:ring-ring/20 focus:border-ring font-[family-name:var(--font-sans)] bg-card text-foreground placeholder:text-muted-foreground",
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
