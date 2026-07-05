import { cn } from "@/lib/utils";
import { cva, type VariantProps } from "class-variance-authority";
import type { HTMLAttributes } from "react";

const badgeVariants = cva("font-medium rounded-full inline-flex items-center gap-1", {
  variants: {
    variant: {
      default: "bg-muted text-muted-foreground",
      outline: "border border-border text-foreground",
      solid: "bg-foreground text-background",
      surface: "bg-accent-soft text-primary-ink border border-primary/20",
    },
    size: {
      sm: "px-2 py-0.5 text-xs",
      md: "px-2.5 py-1 text-sm",
      lg: "px-3 py-1.5 text-base",
    },
  },
  defaultVariants: {
    variant: "default",
    size: "md",
  },
});

interface BadgeProps
  extends HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

const Badge = ({ className, variant, size, ...props }: BadgeProps) => (
  <span
    className={cn(badgeVariants({ variant, size, className }))}
    {...props}
  />
);

export { Badge, badgeVariants };
