import { type ReactNode, useEffect, Suspense } from "react";
import { useRouterState } from "@tanstack/react-router";
import { motion, AnimatePresence } from "framer-motion";
import { Sidebar } from "./Sidebar";
import { Header } from "./Header";
import { RouterProgressBar } from "@/components/ui/RouterProgressBar";
import { Toaster } from "@/components/ui/Toaster";
import { sseClient } from "@/lib/sse";
import { useAuthStore } from "@/store/auth.store";
import { useUIStore } from "@/store/ui.store";
import { useNotificationsRealtime } from "@/hooks/useNotificationsRealtime";

// ── Skeleton shown while a lazy page chunk is downloading ─────────────────────
function PageSkeleton() {
  return (
    <div className="animate-pulse space-y-4 p-6">
      <div className="h-8 w-48 rounded-xl bg-slate-100 dark:bg-white/10" />
      <div className="h-4 w-72 rounded-lg bg-slate-100 dark:bg-white/10" />
      <div className="mt-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="h-24 rounded-2xl bg-slate-100 dark:bg-white/10" />
        ))}
      </div>
    </div>
  );
}

// ── Page transition wrapper ───────────────────────────────────────────────────
function PageTransition({ children }: { children: ReactNode }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  return (
    <AnimatePresence mode="wait" initial={false}>
      <motion.div
        key={pathname}
        initial={{ opacity: 0, y: 6 }}
        animate={{ opacity: 1, y: 0 }}
        exit={{ opacity: 0, y: -4 }}
        transition={{ duration: 0.16, ease: "easeOut" }}
      >
        {children}
      </motion.div>
    </AnimatePresence>
  );
}

// ── AppShell ──────────────────────────────────────────────────────────────────
interface Props {
  children: ReactNode;
}

export function AppShell({ children }: Props) {
  const { isAuthenticated, accessToken } = useAuthStore();
  const { sidebarOpen, closeSidebar } = useUIStore();

  useNotificationsRealtime();

  useEffect(() => {
    if (!isAuthenticated || !accessToken) {
      sseClient.disconnect();
      return;
    }
    sseClient.reconnect();
    return () => sseClient.disconnect();
  }, [isAuthenticated, accessToken]);

  return (
    <>
      {/* Global navigation progress bar — sits above everything */}
      <RouterProgressBar />
      <Toaster />

      <div className="flex min-h-dvh">
        <Sidebar />

        {/* Mobile drawer backdrop — only below lg, when the drawer is open */}
        <AnimatePresence>
          {sidebarOpen && (
            <motion.div
              key="sidebar-backdrop"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.2 }}
              onClick={closeSidebar}
              className="fixed inset-0 z-30 bg-slate-900/40 backdrop-blur-sm lg:hidden"
            />
          )}
        </AnimatePresence>

        {/* Right panel — header card + content card stacked vertically.
            The sidebar is off-canvas below lg, so no left margin there. */}
        <div className="flex min-w-0 flex-1 flex-col gap-3 p-3 sm:p-4 lg:ml-[var(--sidebar-width)]">
          <Header />

          <main className="flex-1 overflow-auto glass-card p-4 sm:p-6 lg:p-8">
            {/*
              Suspense here (not at root) keeps Sidebar + Header visible while
              a lazy page chunk is downloading. PageSkeleton fills the content
              card instead of replacing the whole screen.
            */}
            <Suspense fallback={<PageSkeleton />}>
              <PageTransition>
                {children}
              </PageTransition>
            </Suspense>
          </main>
        </div>
      </div>
    </>
  );
}
