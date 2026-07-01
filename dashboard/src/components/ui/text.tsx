import { cn } from "@/lib/utils";
import { cva, type VariantProps } from "class-variance-authority";
import type { ElementType, HTMLAttributes } from "react";

const textVariants = cva("", {
  variants: {
    as: {
      p: "font-[family-name:var(--font-sans)] text-base",
      display:
        "font-[family-name:var(--font-head)] text-5xl lg:text-6xl font-medium tracking-tight leading-[1.05] [&_em]:font-[family-name:var(--font-serif)] [&_em]:italic [&_em]:font-normal [&_em]:text-primary-ink",
      h1: "font-[family-name:var(--font-head)] text-4xl lg:text-5xl font-medium tracking-tight [&_em]:font-[family-name:var(--font-serif)] [&_em]:italic [&_em]:font-normal [&_em]:text-primary-ink",
      h2: "font-[family-name:var(--font-head)] text-3xl lg:text-4xl font-medium tracking-tight [&_em]:font-[family-name:var(--font-serif)] [&_em]:italic [&_em]:font-normal [&_em]:text-primary-ink",
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

const tags: Record<NonNullable<TextProps["as"]>, ElementType> = {
  p: "p",
  display: "h1",
  h1: "h1",
  h2: "h2",
  h3: "h3",
  h4: "h4",
  h5: "h5",
  span: "span",
};

export function Text({ className, as, ...props }: TextProps) {
  const Tag = tags[as ?? "p"];
  return <Tag className={cn(textVariants({ as }), className)} {...props} />;
}
