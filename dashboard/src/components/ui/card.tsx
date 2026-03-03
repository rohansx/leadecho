import { cn } from "@/lib/utils";
import type { HTMLAttributes } from "react";

type CardProps = HTMLAttributes<HTMLDivElement>;

const Card = ({ className, ...props }: CardProps) => (
  <div
    className={cn(
      "border-2 border-border rounded shadow-md transition-all bg-card text-card-foreground",
      className,
    )}
    {...props}
  />
);

const CardHeader = ({ className, ...props }: CardProps) => (
  <div
    className={cn("flex flex-col justify-start p-4", className)}
    {...props}
  />
);

const CardTitle = ({ className, ...props }: CardProps) => (
  <h3
    className={cn(
      "font-[family-name:var(--font-head)] text-2xl font-medium mb-2",
      className,
    )}
    {...props}
  />
);

const CardDescription = ({ className, ...props }: CardProps) => (
  <p
    className={cn("text-muted-foreground font-[family-name:var(--font-sans)]", className)}
    {...props}
  />
);

const CardContent = ({ className, ...props }: CardProps) => (
  <div className={cn("p-4", className)} {...props} />
);

const CardFooter = ({ className, ...props }: CardProps) => (
  <div
    className={cn("flex items-center p-4 border-t-2 border-border", className)}
    {...props}
  />
);

export { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter };
