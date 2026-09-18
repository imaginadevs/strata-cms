import { forwardRef } from "react";
import { motion, useReducedMotion, type HTMLMotionProps } from "framer-motion";
import { cn } from "@/lib/utils";
import { duration, easing, spring } from "@/lib/motion";

export interface CardProps extends HTMLMotionProps<"div"> {
  /** Adds a subtle lift + shadow on hover. Use for clickable/stat cards. */
  interactive?: boolean;
}

/**
 * Frosted-glass surface. Fades/slides in on mount (reduced-motion safe) and,
 * when `interactive`, lifts slightly on hover.
 */
export const Card = forwardRef<HTMLDivElement, CardProps>(
  ({ className, interactive, ...props }, ref) => {
    const reduce = useReducedMotion();

    return (
      <motion.div
        ref={ref}
        className={cn("glass-card p-5", interactive && "cursor-pointer", className)}
        initial={reduce ? false : { opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: duration.base, ease: easing.out }}
        whileHover={
          interactive && !reduce
            ? { y: -3, transition: spring.press }
            : undefined
        }
        {...props}
      />
    );
  }
);
Card.displayName = "Card";

export const CardHeader = forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div ref={ref} className={cn("mb-4 flex items-center justify-between", className)} {...props} />
  )
);
CardHeader.displayName = "CardHeader";

export const CardTitle = forwardRef<HTMLHeadingElement, React.HTMLAttributes<HTMLHeadingElement>>(
  ({ className, ...props }, ref) => (
    <h3
      ref={ref}
      className={cn("text-base font-semibold text-slate-900 dark:text-slate-100", className)}
      {...props}
    />
  )
);
CardTitle.displayName = "CardTitle";
