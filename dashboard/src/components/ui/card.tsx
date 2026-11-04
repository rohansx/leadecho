import { cn } from "@/lib/utils";
import type { HTMLAttributes } from "react";

type CardProps = HTMLAttributes<HTMLDivElement>;

const Card = ({ className, ...props }: CardProps) => (
  <div
    className={cn(
      "glass-card rounded-xl text-card-foreground shadow-xs",
      className,
    )}
    {...props}
  />
);

const CardHeader = ({ className, ...props }: CardProps) => (
  <div
    className={cn("flex flex-col justify-start gap-0.5 p-5 pb-3", className)}
    {...props}
  />
);

const CardTitle = ({ className, ...props }: CardProps) => (
  <h3
    className={cn(
      "font-[family-name:var(--font-head)] text-[15px] font-semibold tracking-tight leading-6",
      className,
    )}
    {...props}
  />
);

const CardDescription = ({ className, ...props }: CardProps) => (
  <p
    className={cn("text-sm text-muted-foreground font-[family-name:var(--font-sans)]", className)}
    {...props}
  />
);

const CardContent = ({ className, ...props }: CardProps) => (
  <div className={cn("p-5 pt-2", className)} {...props} />
);

const CardFooter = ({ className, ...props }: CardProps) => (
  <div
    className={cn("flex items-center px-5 py-3 border-t border-border", className)}
    {...props}
  />
);

export { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter };
