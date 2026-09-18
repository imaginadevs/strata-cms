import type { Transition, Variants } from "framer-motion";

/**
 * Centralized motion tokens.
 *
 * Import from here instead of hard-coding durations/easings inline, so every
 * animation across the app shares the same "soft" feel. Pair with framer-motion's
 * `useReducedMotion()` to disable motion for users who ask for it.
 */

/** Cubic-bezier easings for duration-based transitions. */
export const easing = {
  /** Soft ease-out — good default for enters. */
  out: [0.16, 1, 0.3, 1] as const,
  /** Symmetric ease-in-out. */
  inOut: [0.4, 0, 0.2, 1] as const,
};

/** Duration scale (seconds). */
export const duration = {
  fast: 0.15,
  base: 0.2,
  slow: 0.3,
};

/** Spring presets — physical, "soft" feel for interactive elements. */
export const spring = {
  /** Snappy tap/press feedback (buttons). */
  press: { type: "spring", stiffness: 400, damping: 25, mass: 0.6 },
  /** Gentle settle (cards, panels). */
  soft: { type: "spring", stiffness: 260, damping: 30 },
  /** Bouncier accent (toasts, badges). */
  bounce: { type: "spring", stiffness: 500, damping: 18 },
} satisfies Record<string, Transition>;

/** Standard mount/unmount variants (modals, cards, list items). */
export const fadeInUp: Variants = {
  hidden: { opacity: 0, y: 6 },
  visible: {
    opacity: 1,
    y: 0,
    transition: { duration: duration.fast, ease: easing.out },
  },
  exit: {
    opacity: 0,
    y: -4,
    transition: { duration: duration.fast, ease: easing.out },
  },
};

/**
 * Hover/press presets for interactive surfaces (buttons, clickable cards).
 * Spread onto a `motion` element; gate with `useReducedMotion()` when needed.
 */
export const interactive = {
  whileHover: { y: -1 },
  whileTap: { scale: 0.96 },
  transition: spring.press,
};
