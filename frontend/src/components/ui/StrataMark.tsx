import { forwardRef } from "react";

import { cn } from "@/lib/utils";

/**
 * StrataMark — the Strata "stacked layers" brand mark as an inline SVG.
 *
 * Multi-color by design (brand-blue strata). Dark mode is handled with
 * Tailwind `dark:` fill utilities (the app toggles `.dark` on <html>), not a
 * `prefers-color-scheme` media query. Size via className (e.g. `h-4 w-4`).
 */
export const StrataMark = forwardRef<SVGSVGElement, React.SVGProps<SVGSVGElement>>(
  ({ className, ...props }, ref) => (
    <svg
      ref={ref}
      viewBox="0 0 128 128"
      role="img"
      aria-label="Strata"
      className={cn("shrink-0", className)}
      {...props}
    >
      {/* bottom layer (foundation) */}
      <polygon className="fill-brand-700 dark:fill-brand-500" points="56,76 94,92 56,108 18,92" />
      {/* middle layer */}
      <polygon className="fill-brand-600 dark:fill-brand-400" points="64,48 102,64 64,80 26,64" />
      {/* top layer (newest content) */}
      <polygon className="fill-brand-400 dark:fill-brand-300" points="72,20 110,36 72,52 34,36" />
    </svg>
  ),
);

StrataMark.displayName = "StrataMark";
