import { Search, LogOut, User, ChevronDown, Sun, Moon, Menu } from "lucide-react";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { motion, useReducedMotion } from "framer-motion";
import { useNavigate } from "@tanstack/react-router";
import { useAuthStore } from "@/store/auth.store";
import { useThemeStore } from "@/store/theme.store";
import { useUIStore } from "@/store/ui.store";
import { post } from "@/lib/api";
import { initials } from "@/lib/utils";
import { spring } from "@/lib/motion";
import { NotificationsBell } from "./NotificationsBell";

function ThemeToggle() {
  const { theme, toggle } = useThemeStore();
  const reduce = useReducedMotion();
  const isDark = theme === "dark";

  return (
    <button
      onClick={toggle}
      aria-label={isDark ? "Switch to light mode" : "Switch to dark mode"}
      className="relative flex h-9 w-9 items-center justify-center rounded-xl
                 text-slate-500 transition hover:bg-slate-100
                 dark:text-slate-300 dark:hover:bg-white/10"
    >
      <motion.span
        key={theme}
        initial={reduce ? false : { rotate: -90, opacity: 0, scale: 0.6 }}
        animate={{ rotate: 0, opacity: 1, scale: 1 }}
        transition={spring.soft}
        className="flex items-center justify-center"
      >
        {isDark ? <Sun size={17} strokeWidth={1.8} /> : <Moon size={17} strokeWidth={1.8} />}
      </motion.span>
    </button>
  );
}

export function Header() {
  const { user, clearSession } = useAuthStore();
  const { toggleSidebar } = useUIStore();
  const navigate = useNavigate();

  const handleLogout = async () => {
    try {
      await post("/auth/logout");
    } catch {
      // proceed even if server logout fails
    }
    clearSession();
    navigate({ to: "/login" });
  };

  return (
    <header className="flex h-[var(--header-height)] shrink-0 items-center
                       gap-4 rounded-2xl border border-white/50 bg-white/70 px-6
                       shadow-card backdrop-blur-xl
                       dark:border-white/10 dark:bg-slate-900/50">
      {/* Hamburger — opens the sidebar drawer below lg */}
      <button
        onClick={toggleSidebar}
        aria-label="Open navigation menu"
        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl
                   text-slate-500 transition hover:bg-slate-100
                   dark:text-slate-300 dark:hover:bg-white/10 lg:hidden"
      >
        <Menu size={18} strokeWidth={1.8} />
      </button>

      {/* Search */}
      <div className="relative flex-1 max-w-sm">
        <Search size={15} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
        <input
          type="search"
          placeholder="Search…"
          className="h-9 w-full rounded-xl border border-slate-200 bg-slate-50/70
                     pl-9 pr-4 text-sm text-slate-700 placeholder:text-slate-400
                     focus:border-brand-400 focus:bg-white focus:outline-none focus:ring-2 focus:ring-brand-100
                     transition-all
                     dark:border-white/10 dark:bg-white/5 dark:text-slate-100 dark:placeholder:text-slate-500
                     dark:focus:border-brand-500/60 dark:focus:bg-white/10 dark:focus:ring-brand-500/20"
        />
      </div>

      <div className="flex items-center gap-2 ml-auto">
        {/* Theme toggle */}
        <ThemeToggle />

        <NotificationsBell />

        {/* User menu */}
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button className="flex items-center gap-2.5 rounded-xl px-2 py-1.5
                               text-sm text-slate-700 transition hover:bg-slate-100
                               dark:text-slate-200 dark:hover:bg-white/10">
              <div className="flex h-7 w-7 items-center justify-center rounded-full
                              bg-brand-600 text-xs font-semibold text-white">
                {user ? initials(`${user.first_name} ${user.last_name}`) : "?"}
              </div>
              <span className="font-medium text-slate-700 dark:text-slate-200">
                {user?.first_name ?? "Account"}
              </span>
              <ChevronDown size={13} className="text-slate-400" />
            </button>
          </DropdownMenu.Trigger>

          <DropdownMenu.Portal>
            <DropdownMenu.Content
              align="end"
              sideOffset={8}
              className="min-w-48 animate-slide-up rounded-xl border border-slate-200
                         bg-white p-1.5 shadow-lg
                         dark:border-white/10 dark:bg-slate-800"
            >
              <DropdownMenu.Item
                onSelect={() => navigate({ to: "/profile" })}
                className="flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2
                           text-sm text-slate-700 outline-none hover:bg-slate-100
                           dark:text-slate-200 dark:hover:bg-white/10"
              >
                <User size={15} />
                My Profile
              </DropdownMenu.Item>
              <DropdownMenu.Separator className="my-1 h-px bg-slate-100 dark:bg-white/10" />
              <DropdownMenu.Item
                onSelect={handleLogout}
                className="flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2
                           text-sm text-red-600 outline-none hover:bg-red-50
                           dark:text-red-400 dark:hover:bg-red-500/10"
              >
                <LogOut size={15} />
                Sign out
              </DropdownMenu.Item>
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      </div>
    </header>
  );
}
