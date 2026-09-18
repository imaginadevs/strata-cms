import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";

export type Theme = "light" | "dark";

interface ThemeState {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  toggle: () => void;
}

/** Apply (or remove) the `dark` class on <html>. */
function applyTheme(theme: Theme) {
  const root = document.documentElement;
  root.classList.toggle("dark", theme === "dark");
  root.style.colorScheme = theme;
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set) => ({
      theme: "light",
      setTheme: (theme) => {
        applyTheme(theme);
        set({ theme });
      },
      toggle: () =>
        set((state) => {
          const next: Theme = state.theme === "dark" ? "light" : "dark";
          applyTheme(next);
          return { theme: next };
        }),
    }),
    {
      name: "cms-theme",
      storage: createJSONStorage(() => localStorage),
      // Re-apply the class after the persisted value is rehydrated.
      onRehydrateStorage: () => (state) => {
        if (state) applyTheme(state.theme);
      },
    }
  )
);

/**
 * Apply the persisted theme before first paint to avoid a flash.
 * Call once from main.tsx prior to rendering. (persist rehydrates
 * synchronously from localStorage, so getState() is already correct here.)
 */
export function initTheme() {
  applyTheme(useThemeStore.getState().theme);
}
