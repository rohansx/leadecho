import { forwardRef } from "react";
import type { TextareaHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement>;

const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, ...props }, ref) => (
    <textarea
      className={cn(
        "px-4 py-2 w-full rounded border-2 border-border shadow-md transition focus:outline-hidden focus:shadow-xs font-[family-name:var(--font-sans)] bg-background text-foreground placeholder:text-muted-foreground resize-y min-h-20",
        className,
      )}
      ref={ref}
      {...props}
    />
  ),
);
Textarea.displayName = "Textarea";

export { Textarea };
