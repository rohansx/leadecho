import { forwardRef } from "react";
import type { TextareaHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement>;

const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, ...props }, ref) => (
    <textarea
      className={cn(
        "px-3.5 py-2 w-full rounded-lg border border-border shadow-xs transition-shadow focus:outline-hidden focus:ring-2 focus:ring-ring/30 focus:border-ring font-[family-name:var(--font-sans)] bg-card text-foreground placeholder:text-muted-foreground resize-y min-h-20",
        className,
      )}
      ref={ref}
      {...props}
    />
  ),
);
Textarea.displayName = "Textarea";

export { Textarea };
