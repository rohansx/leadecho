import { cn } from "@/lib/utils";
import { cva, type VariantProps } from "class-variance-authority";
import type { ElementType, HTMLAttributes } from "react";

const textVariants = cva("", {
  variants: {
    as: {
      p: "font-[family-name:var(--font-sans)] text-base",
      h1: "font-[family-name:var(--font-head)] text-4xl lg:text-5xl font-bold",
      h2: "font-[family-name:var(--font-head)] text-3xl lg:text-4xl font-semibold",
      h3: "font-[family-name:var(--font-head)] text-2xl font-medium",
      h4: "font-[family-name:var(--font-head)] text-xl font-normal",
      h5: "font-[family-name:var(--font-head)] text-lg font-normal",
      span: "font-[family-name:var(--font-sans)] text-base",
    },
  },
  defaultVariants: {
    as: "p",
  },
});

interface TextProps
  extends Omit<HTMLAttributes<HTMLElement>, "className">,
    VariantProps<typeof textVariants> {
  className?: string;
}

export function Text({ className, as, ...props }: TextProps) {
  const Tag: ElementType = as ?? "p";
  return <Tag className={cn(textVariants({ as }), className)} {...props} />;
}
