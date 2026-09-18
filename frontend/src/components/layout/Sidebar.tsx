import { useEffect } from "react";
import { Link, useRouterState } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { motion, AnimatePresence } from "framer-motion";
import {
  LayoutDashboard,
  Database,
  Users,
  ScrollText,
  Settings,
  ChevronLeft,
  Globe,
  Lock,
  Plus,
  SlidersHorizontal,
  Shield,
  Workflow,
} from "lucide-react";
import { useTenantStore } from "@/store/tenant.store";
import { useUIStore } from "@/store/ui.store";
import { get } from "@/lib/api";
import type { Collection, PaginatedResponse } from "@/types";

// ── Routes that belong to the "settings" section ─────────────────────────────
const SETTINGS_ROOTS = ["/collections", "/users", "/roles", "/audit", "/settings"];

function isInSettings(pathname: string) {
  return SETTINGS_ROOTS.some(
    (r) => pathname === r || pathname.startsWith(r + "/")
  );
}

// ── Shared nav-item component ─────────────────────────────────────────────────
function NavItem({
  to,
  label,
  icon: Icon,
  iconColor,
  exact = false,
}: {
  to: string;
  label: string;
  icon: React.ElementType;
  iconColor?: string;
  exact?: boolean;
}) {
  return (
    <li>
      <Link
        to={to}
        activeOptions={exact ? { exact: true } : undefined}
        className="group flex items-center gap-3 rounded-xl px-3 py-2.5
                   transition-colors hover:bg-slate-100/70 dark:hover:bg-white/5
                   data-[status=active]:bg-brand-50 dark:data-[status=active]:bg-brand-500/15"
      >
        <div
          className="flex h-8 w-8 shrink-0 items-center justify-center
                     rounded-lg transition-colors"
          style={
            iconColor
              ? { backgroundColor: `${iconColor}18`, color: iconColor }
              : undefined
          }
        >
          <span
            className={
              iconColor
                ? undefined
                : "flex h-full w-full items-center justify-center rounded-lg bg-slate-100 text-slate-500 dark:bg-white/10 dark:text-slate-400 transition-colors group-data-[status=active]:bg-brand-700 group-data-[status=active]:text-white group-data-[status=active]:shadow-sm"
            }
          >
            <Icon size={15} strokeWidth={1.8} />
          </span>
        </div>
        <span className="truncate text-sm font-medium text-slate-600 dark:text-slate-300 transition-colors group-data-[status=active]:text-brand-800 dark:group-data-[status=active]:text-brand-200">
          {label}
        </span>
      </Link>
    </li>
  );
}

// ── Content-mode nav ──────────────────────────────────────────────────────────
function ContentNav() {
  const { data: collections = [], isLoading } = useQuery({
    queryKey: ["collections"],
    queryFn: () =>
      get<PaginatedResponse<Collection>>("/collections").then(
        (r) => r.data ?? []
      ),
  });

  // Hidden collections (is_hidden) are kept out of the content menu.
  const visibleCollections = collections.filter((c) => !c.is_hidden);

  return (
    <nav className="flex-1 overflow-y-auto px-3 py-3 space-y-1">
      {/* BI */}
      <div>
        <p className="mb-1 px-3 text-[10px] font-semibold uppercase tracking-widest text-slate-400">
          BI
        </p>
        <ul className="space-y-0.5">
          <NavItem to="/" label="Dashboard" icon={LayoutDashboard} exact />
        </ul>
      </div>

      {/* Dynamic collections */}
      {visibleCollections.length > 0 && (
        <div className="pt-3">
          <p className="mb-1 px-3 text-[10px] font-semibold uppercase tracking-widest text-slate-400">
            Content
          </p>
          <ul className="space-y-0.5">
            {visibleCollections.map((c) => (
              <NavItem
                key={c.id}
                to={`/content/${c.name}`}
                label={c.display_name}
                icon={c.is_system ? Lock : Database}
                iconColor={c.color ?? undefined}
              />
            ))}
          </ul>
        </div>
      )}

      {/* Placeholder while loading */}
      {isLoading && (
        <div className="pt-3">
          <p className="mb-2 px-3 text-[10px] font-semibold uppercase tracking-widest text-slate-400">
            Content
          </p>
          <div className="space-y-1.5 px-3">
            {[72, 56, 64].map((w) => (
              <div
                key={w}
                className="h-8 animate-pulse rounded-lg bg-slate-100 dark:bg-white/10"
                style={{ width: `${w}%` }}
              />
            ))}
          </div>
        </div>
      )}

      {/* Automations */}
      <div className="pt-3">
        <p className="mb-1 px-3 text-[10px] font-semibold uppercase tracking-widest text-slate-400">
          Automations
        </p>
        <ul className="space-y-0.5">
          <NavItem to="/workflows" label="Workflows" icon={Workflow} />
        </ul>
      </div>
    </nav>
  );
}

// ── Settings-mode nav ─────────────────────────────────────────────────────────
const SETTINGS_NAV = [
  { to: "/collections",  label: "Collections",   icon: Database },
  { to: "/users",        label: "Users",          icon: Users },
  { to: "/roles",        label: "Roles",          icon: Shield },
  { to: "/audit",        label: "Audit Logs",     icon: ScrollText },
  { to: "/settings",     label: "Configuration",  icon: SlidersHorizontal },
] as const;

function SettingsNav() {
  return (
    <nav className="flex-1 overflow-y-auto px-3 py-3">
      {/* Back to content */}
      <Link
        to="/"
        className="mb-4 flex items-center gap-2 rounded-xl px-3 py-2
                   text-sm font-medium text-slate-500 transition hover:bg-slate-100/70 dark:text-slate-400 dark:hover:bg-white/5 hover:text-slate-800 dark:hover:text-slate-100"
      >
        <ChevronLeft size={16} />
        Content
      </Link>

      <p className="mb-1 px-3 text-[10px] font-semibold uppercase tracking-widest text-slate-400">
        Settings
      </p>
      <ul className="space-y-0.5">
        {SETTINGS_NAV.map(({ to, label, icon }) => (
          <NavItem key={to} to={to} label={label} icon={icon} />
        ))}
      </ul>
    </nav>
  );
}

// ── Sidebar ───────────────────────────────────────────────────────────────────
export function Sidebar() {
  const { tenant } = useTenantStore();
  const { sidebarOpen, closeSidebar } = useUIStore();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const settingsMode = isInSettings(pathname);

  // Close the mobile drawer whenever the route changes.
  useEffect(() => {
    closeSidebar();
  }, [pathname, closeSidebar]);

  return (
    <aside
      className={`fixed inset-y-0 left-0 z-40 flex w-[var(--sidebar-width)] flex-col
                 border-r border-slate-200 bg-white
                 transition-transform duration-300 ease-out lg:translate-x-0
                 dark:border-white/10 dark:bg-slate-900
                 lg:border-white/40 lg:bg-white/70 lg:backdrop-blur-xl
                 dark:lg:bg-slate-900/50
                 ${sidebarOpen ? "translate-x-0" : "-translate-x-full"}`}
    >
      {/* Logo / brand */}
      <div
        className="flex h-[var(--header-height)] shrink-0 items-center gap-2.5 px-5
                   border-b border-slate-100 dark:border-white/10"
      >
        <div
          className="flex h-7 w-7 shrink-0 items-center justify-center
                     rounded-lg bg-brand-600 text-[11px] font-bold text-white shadow-sm"
        >
          {tenant?.name?.[0]?.toUpperCase() ?? "C"}
        </div>
        <span className="text-sm font-bold text-slate-800 dark:text-slate-100 tracking-tight">
          {tenant?.name ?? "My Workspace"}
        </span>
      </div>

      {/* Nav — animated between content and settings modes */}
      <div className="relative flex-1 overflow-hidden">
        <AnimatePresence mode="wait" initial={false}>
          {settingsMode ? (
            <motion.div
              key="settings"
              className="absolute inset-0 flex flex-col"
              initial={{ opacity: 0, x: 20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 20 }}
              transition={{ duration: 0.18, ease: "easeOut" }}
            >
              <SettingsNav />
            </motion.div>
          ) : (
            <motion.div
              key="content"
              className="absolute inset-0 flex flex-col"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -20 }}
              transition={{ duration: 0.18, ease: "easeOut" }}
            >
              <ContentNav />
            </motion.div>
          )}
        </AnimatePresence>
      </div>

      {/* Footer — Settings entry (only in content mode) / version */}
      <div className="shrink-0 border-t border-slate-100 dark:border-white/10 px-3 py-3">
        {!settingsMode ? (
          <Link
            to="/collections"
            className="flex w-full items-center gap-3 rounded-xl px-3 py-2.5
                       transition-colors hover:bg-slate-100/70 dark:hover:bg-white/5"
          >
            <div
              className="flex h-8 w-8 shrink-0 items-center justify-center
                         rounded-lg bg-slate-100 text-slate-500 dark:bg-white/10 dark:text-slate-400"
            >
              <Settings size={15} strokeWidth={1.8} />
            </div>
            <span className="text-sm font-medium text-slate-600 dark:text-slate-300">Settings</span>
            <Plus size={12} className="ml-auto text-slate-300" />
          </Link>
        ) : (
          <div className="flex items-center gap-2 px-3 text-xs text-slate-400">
            <Globe size={12} />
            <span className="font-medium">CMS</span>
            <span className="ml-auto font-mono opacity-50">v0.1.0</span>
          </div>
        )}
      </div>
    </aside>
  );
}
