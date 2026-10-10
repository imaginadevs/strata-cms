import { useEffect, useState } from "react";
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
  ChevronDown,
  ChevronRight,
  Lock,
  Plus,
  Search,
  Folder,
  LayoutGrid,
  SlidersHorizontal,
  Shield,
  Workflow,
  Building2,
  KeyRound,
} from "lucide-react";
import { StrataMark } from "@/components/ui/StrataMark";
import { useTenantStore } from "@/store/tenant.store";
import { useAuthStore } from "@/store/auth.store";
import { useUIStore } from "@/store/ui.store";
import { get } from "@/lib/api";
import type { Collection, CollectionGroup, PaginatedResponse } from "@/types";

// ── Routes that belong to the "settings" section ─────────────────────────────
const SETTINGS_ROOTS = ["/collections", "/users", "/tenants", "/roles", "/api-tokens", "/audit", "/settings"];

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

// ── Collapsed-group persistence (per browser) ─────────────────────────────────
const COLLAPSE_KEY = "cms.nav.collapsed-groups";

function useCollapsedGroups() {
  const [collapsed, setCollapsed] = useState<Set<string>>(() => {
    try {
      const raw = localStorage.getItem(COLLAPSE_KEY);
      return new Set<string>(raw ? JSON.parse(raw) : []);
    } catch {
      return new Set<string>();
    }
  });
  const toggle = (id: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      try {
        localStorage.setItem(COLLAPSE_KEY, JSON.stringify([...next]));
      } catch {
        /* ignore private-mode write errors */
      }
      return next;
    });
  return { collapsed, toggle };
}

function CollectionLink({ c }: { c: Collection }) {
  return (
    <NavItem
      to={`/content/${c.name}`}
      label={c.display_name}
      icon={c.is_system ? Lock : Database}
      iconColor={c.color ?? undefined}
    />
  );
}

// ── Collapsible group section ─────────────────────────────────────────────────
function GroupSection({
  group,
  items,
  open,
  onToggle,
}: {
  group: CollectionGroup;
  items: Collection[];
  open: boolean;
  onToggle: () => void;
}) {
  const color = group.color ?? undefined;
  return (
    <div className="pt-2">
      <button
        type="button"
        onClick={onToggle}
        className="flex w-full items-center gap-2 rounded-lg px-3 py-1.5 text-left transition-colors hover:bg-slate-100/70 dark:hover:bg-white/5"
      >
        {open ? (
          <ChevronDown size={13} className="shrink-0 text-slate-400" />
        ) : (
          <ChevronRight size={13} className="shrink-0 text-slate-400" />
        )}
        <span
          className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md"
          style={
            color
              ? { backgroundColor: `${color}22`, color }
              : undefined
          }
        >
          <Folder size={12} className={color ? undefined : "text-slate-400"} />
        </span>
        <span className="flex-1 truncate text-[11px] font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
          {group.name}
        </span>
        <span className="text-[10px] font-medium text-slate-400">{items.length}</span>
      </button>
      <AnimatePresence initial={false}>
        {open && (
          <motion.ul
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.15 }}
            className="space-y-0.5 overflow-hidden pl-3"
          >
            {items.map((c) => (
              <CollectionLink key={c.id} c={c} />
            ))}
          </motion.ul>
        )}
      </AnimatePresence>
    </div>
  );
}

// ── Content-mode nav ──────────────────────────────────────────────────────────
function ContentNav() {
  const { data: collections = [], isLoading } = useQuery({
    queryKey: ["collections"],
    queryFn: () =>
      get<PaginatedResponse<Collection>>("/collections").then((r) => r.data ?? []),
  });
  const { data: groups = [] } = useQuery({
    queryKey: ["collection-groups"],
    queryFn: () => get<CollectionGroup[]>("/collection-groups"),
  });

  const { collapsed, toggle } = useCollapsedGroups();
  const [query, setQuery] = useState("");
  const q = query.trim().toLowerCase();

  // Hidden collections (is_hidden) are kept out of the content menu.
  const visible = collections.filter((c) => !c.is_hidden);
  const matches = (c: Collection) =>
    !q ||
    c.display_name.toLowerCase().includes(q) ||
    c.name.toLowerCase().includes(q);

  const byPosition = (a: Collection, b: Collection) =>
    a.sort - b.sort || a.display_name.localeCompare(b.display_name);

  const sortedGroups = [...groups].sort(
    (a, b) => a.sort - b.sort || a.name.localeCompare(b.name)
  );
  const inGroup = (gid: string) =>
    visible.filter((c) => c.group_id === gid && matches(c)).sort(byPosition);
  const ungrouped = visible
    .filter((c) => !c.group_id && matches(c))
    .sort(byPosition);

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

      {/* Content header + quick search + overview link */}
      <div className="pt-3">
        <div className="mb-1 flex items-center justify-between px-3">
          <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-400">
            Content
          </p>
          <Link
            to="/content"
            className="flex items-center gap-1 text-[10px] font-medium text-slate-400 transition hover:text-brand-600 dark:hover:text-brand-300"
            title="Browse all collections"
          >
            <LayoutGrid size={11} />
            All
          </Link>
        </div>
        {visible.length > 6 && (
          <div className="mb-1 flex items-center gap-2 rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-2.5 py-1.5">
            <Search size={13} className="shrink-0 text-slate-400" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Find a collection…"
              className="w-full bg-transparent text-xs text-slate-700 placeholder:text-slate-400 focus:outline-none dark:text-slate-200"
            />
          </div>
        )}
      </div>

      {/* Loading skeleton */}
      {isLoading && (
        <div className="space-y-1.5 px-3 pt-1">
          {[72, 56, 64].map((w) => (
            <div
              key={w}
              className="h-8 animate-pulse rounded-lg bg-slate-100 dark:bg-white/10"
              style={{ width: `${w}%` }}
            />
          ))}
        </div>
      )}

      {/* Grouped collections */}
      {sortedGroups.map((g) => {
        const items = inGroup(g.id);
        if (q && items.length === 0) return null; // hide empty groups while searching
        return (
          <GroupSection
            key={g.id}
            group={g}
            items={items}
            open={!!q || !collapsed.has(g.id)}
            onToggle={() => toggle(g.id)}
          />
        );
      })}

      {/* Ungrouped collections */}
      {ungrouped.length > 0 && (
        <div className="pt-2">
          {sortedGroups.length > 0 && (
            <p className="mb-1 px-3 text-[10px] font-semibold uppercase tracking-wide text-slate-400">
              Ungrouped
            </p>
          )}
          <ul className="space-y-0.5">
            {ungrouped.map((c) => (
              <CollectionLink key={c.id} c={c} />
            ))}
          </ul>
        </div>
      )}

      {/* Empty search result */}
      {q && ungrouped.length === 0 && sortedGroups.every((g) => inGroup(g.id).length === 0) && (
        <p className="px-3 pt-2 text-xs text-slate-400">No collections match “{query}”.</p>
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
  { to: "/api-tokens",   label: "API Tokens",     icon: KeyRound },
  { to: "/audit",        label: "Audit Logs",     icon: ScrollText },
  { to: "/settings",     label: "Configuration",  icon: SlidersHorizontal },
] as const;

function SettingsNav() {
  const user = useAuthStore((s) => s.user);
  const isSuper = !!user?.is_super_admin || !!user?.roles?.includes("super_admin");

  // Tenants management is super-admin only; insert it after Users.
  const items = isSuper
    ? [
        ...SETTINGS_NAV.slice(0, 2),
        { to: "/tenants", label: "Tenants", icon: Building2 } as const,
        ...SETTINGS_NAV.slice(2),
      ]
    : SETTINGS_NAV;

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
        {items.map(({ to, label, icon }) => (
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
          {tenant?.name?.[0]?.toUpperCase() ?? "S"}
        </div>
        <div className="flex min-w-0 flex-col leading-tight">
          <span className="truncate text-sm font-bold text-slate-800 dark:text-slate-100 tracking-tight">
            {tenant?.name ?? "Workspace"}
          </span>
          <span className="text-[11px] font-medium text-slate-400">Admin</span>
        </div>
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

      {/* Footer — Settings entry (only in content mode) + required attribution */}
      <div className="shrink-0 space-y-1 border-t border-slate-100 dark:border-white/10 px-3 py-3">
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
            <StrataMark className="h-3.5 w-3.5" />
            <span className="font-medium">Strata</span>
            <span className="ml-auto font-mono opacity-50">v0.1.0</span>
          </div>
        )}

        {/*
          Attribution — required by LICENSE (Attribution clause). This notice and
          its link must remain visible in every deployment; do not remove or hide it.
        */}
        <a
          href="https://www.imaginacolombia.com/"
          target="_blank"
          rel="noreferrer"
          className="flex items-center justify-center gap-1.5 rounded-lg px-3 py-1.5
                     text-[11px] text-slate-400 transition hover:text-brand-600 dark:hover:text-brand-300"
        >
          <StrataMark className="h-3 w-3" />
          <span>
            Made with <span className="font-semibold">Strata</span> · Imagina
          </span>
        </a>
      </div>
    </aside>
  );
}
