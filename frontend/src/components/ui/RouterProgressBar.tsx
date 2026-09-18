/**
 * RouterProgressBar
 *
 * A slim top-of-screen bar that appears while TanStack Router is navigating.
 * Uses an indeterminate "fill to 90% then complete" pattern so it always looks
 * responsive even when we can't know true load progress.
 *
 * How it works:
 *  1. Router goes to `status === "pending"` → bar enters, animates 0→85% (spring)
 *  2. Router goes back to `status === "idle"` → bar shoots to 100%, then fades out
 */
import { useEffect, useRef, useState } from "react";
import { useRouterState } from "@tanstack/react-router";
import { motion, AnimatePresence } from "framer-motion";

export function RouterProgressBar() {
  const status = useRouterState({ select: (s) => s.status });
  const pending = status === "pending";

  // Track whether the bar is in "completing" phase (0→85) or "done" phase (→100)
  const [phase, setPhase] = useState<"filling" | "completing" | "hidden">("hidden");
  const timerRef = useRef<ReturnType<typeof setTimeout>>();

  useEffect(() => {
    if (pending) {
      setPhase("filling");
    } else if (phase === "filling") {
      // Navigation done — shoot to 100 then hide
      setPhase("completing");
      timerRef.current = setTimeout(() => setPhase("hidden"), 400);
    }
    return () => clearTimeout(timerRef.current);
  }, [pending]); // eslint-disable-line react-hooks/exhaustive-deps

  const visible = phase !== "hidden";

  return (
    <AnimatePresence>
      {visible && (
        <motion.div
          key="progress"
          className="pointer-events-none fixed left-0 top-0 z-[200] h-[2px] w-full origin-left"
          initial={false}
        >
          <motion.div
            className="h-full bg-brand-500"
            initial={{ scaleX: 0 }}
            animate={{
              scaleX: phase === "completing" ? 1 : 0.82,
              opacity: phase === "completing" ? 0 : 1,
            }}
            transition={
              phase === "completing"
                ? { duration: 0.25, ease: "easeOut" }
                : { duration: 0.6, ease: [0.1, 0.4, 0.6, 1] }
            }
            style={{ originX: 0 }}
          />
        </motion.div>
      )}
    </AnimatePresence>
  );
}
