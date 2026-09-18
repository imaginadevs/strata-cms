---
name: new-component
description: Scaffold a new reusable UI component in src/components/ui following this project's CVA + forwardRef + cn conventions. Use when the user asks to create a new component, button, card, badge, input, or similar primitive.
---

# new-component

Create a new UI primitive under `src/components/ui/` that matches the existing
conventions in this repo.

## Conventions (must follow)

- **Styling:** Tailwind classes only, composed with `cn()` from `@/lib/utils`.
- **Variants:** use `class-variance-authority` (`cva`). Export the cva instance as
  `<name>Variants` so other components (e.g. a motion wrapper) can reuse it.
- **Ref forwarding:** wrap in `forwardRef`, set `Component.displayName`.
- **Props:** extend the correct DOM props type and `VariantProps<typeof …Variants>`.
- **Radius/shadow/brand:** prefer existing tokens — `rounded-xl/2xl`, `shadow-card`,
  `bg-brand-600`, `focus-visible:ring-brand-400` — see `tailwind.config.js`.
- **Motion:** for hover/press feedback, use a `motion.*` element with tokens from
  `@/lib/motion` and gate with `useReducedMotion()`. See `MotionButton.tsx`.

## Steps

1. Ask for the component name and its variants/sizes if not given.
2. Create `src/components/ui/<Name>.tsx` from the template below.
3. Run `npm run type-check` to confirm it compiles.
4. If it has interactive states, offer to add a `Motion<Name>` variant.

## Template

```tsx
import { forwardRef } from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

export const badgeVariants = cva(
  "inline-flex items-center rounded-xl text-xs font-medium transition-colors",
  {
    variants: {
      variant: {
        default: "bg-slate-100 text-slate-700",
        brand: "bg-brand-50 text-brand-700",
      },
    },
    defaultVariants: { variant: "default" },
  }
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

export const Badge = forwardRef<HTMLSpanElement, BadgeProps>(
  ({ className, variant, ...props }, ref) => (
    <span ref={ref} className={cn(badgeVariants({ variant }), className)} {...props} />
  )
);

Badge.displayName = "Badge";
```
