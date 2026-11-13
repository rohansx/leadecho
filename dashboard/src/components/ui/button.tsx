import { cn } from "@/lib/utils";
import { cva, type VariantProps } from "class-variance-authority";
import type { ButtonHTMLAttributes } from "react";
import { forwardRef } from "react";
import { Slot } from "@radix-ui/react-slot";

const buttonVariants = cva(
  "font-[family-name:var(--font-head)] transition-[background-color,border-color,color,box-shadow,transform] duration-150 rounded-lg outline-hidden cursor-pointer font-medium inline-flex justify-center items-center gap-1.5 whitespace-nowrap focus-visible:ring-2 focus-visible:ring-ring/40 active:scale-[0.98] disabled:opacity-50 disabled:cursor-not-allowed disabled:active:scale-100",
  {
    variants: {
      variant: {
        // Ink primary (YC-style); amber is reserved for the brand "accent" variant.
        default: "bg-foreground text-background shadow-xs hover:bg-foreground/85",
        accent: "bg-primary text-primary-foreground shadow-xs hover:bg-primary-hover",
        secondary:
          "bg-secondary text-secondary-foreground hover:bg-accent",
        outline:
          "bg-card border border-border text-foreground shadow-xs hover:bg-accent",
        ghost: "bg-transparent text-foreground hover:bg-accent",
        link: "bg-transparent text-primary-ink hover:underline underline-offset-4 p-0!",
        destructive:
          "bg-destructive text-destructive-foreground shadow-sm hover:opacity-90",
      },
      size: {
        sm: "h-7 px-2.5 text-xs",
        md: "h-8 px-3 text-[13px]",
        lg: "h-10 px-5 text-sm",
        icon: "size-8",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "md",
    },
  },
);

export interface ButtonProps
  extends ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    return (
      <Comp
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    );
  },
);
Button.displayName = "Button";

export { Button, buttonVariants };
