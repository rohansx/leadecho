import { forwardRef } from "react";
import type { InputHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

type InputProps = InputHTMLAttributes<HTMLInputElement>;

const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, type = "text", ...props }, ref) => (
    <input
      type={type}
      className={cn(
        "px-4 py-2 w-full rounded border-2 border-border shadow-md transition focus:outline-hidden focus:shadow-xs font-[family-name:var(--font-sans)] bg-background text-foreground placeholder:text-muted-foreground",
        props["aria-invalid"] &&
          "border-destructive text-destructive shadow-xs shadow-destructive",
        className,
      )}
      ref={ref}
      {...props}
    />
  ),
);
Input.displayName = "Input";

export { Input };
