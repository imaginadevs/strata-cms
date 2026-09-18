import { forwardRef, type ReactNode } from "react";
import { motion, useReducedMotion, type HTMLMotionProps } from "framer-motion";
import { type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";
import { spring } from "@/lib/motion";
import { buttonVariants } from "./Button";

/**
 * Button with spring-based hover/press feedback.
 *
 * Shares the exact CVA variants of {@link Button}, so `variant`/`size` behave
 * identically. Reach for this when a button deserves a softer, physical press
 * (primary CTAs, cards); use the plain `Button` for dense/utility controls.
 *
 * Motion is disabled automatically when the user prefers reduced motion.
 * (framer-motion sets `transform` inline, which supersedes the CVA's
 * `active:scale-*` — so there's no double-transform to worry about.)
 */
export interface MotionButtonProps
  extends Omit<HTMLMotionProps<"button">, "children">,
    VariantProps<typeof buttonVariants> {
  loading?: boolean;
  children?: ReactNode;
}

export const MotionButton = forwardRef<HTMLButtonElement, MotionButtonProps>(
  ({ className, variant, size, loading, children, disabled, ...props }, ref) => {
    const reduce = useReducedMotion();

    return (
      <motion.button
        ref={ref}
        className={cn(buttonVariants({ variant, size }), className)}
        disabled={loading || disabled}
        whileHover={reduce ? undefined : { y: -1 }}
        whileTap={reduce ? undefined : { scale: 0.96 }}
        transition={spring.press}
        {...props}
      >
        {loading && (
          <span className="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
        )}
        {children}
      </motion.button>
    );
  }
);

MotionButton.displayName = "MotionButton";
