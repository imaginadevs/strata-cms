import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import { immer } from "zustand/middleware/immer";
import type { Tenant } from "@/types";

interface TenantState {
  tenant: Tenant | null;
  activeLanguage: string;

  setTenant: (tenant: Tenant) => void;
  setLanguage: (lang: string) => void;
  clearTenant: () => void;
}

export const useTenantStore = create<TenantState>()(
  persist(
    immer((set) => ({
      tenant: null,
      activeLanguage: "en",

      setTenant: (tenant: Tenant) => {
        set((state) => {
          state.tenant = tenant;
          state.activeLanguage = tenant.default_language;
        });
      },

      setLanguage: (lang: string) => {
        set((state) => {
          state.activeLanguage = lang;
        });
      },

      clearTenant: () => {
        set((state) => {
          state.tenant = null;
        });
      },
    })),
    {
      name: "cms-tenant",
      storage: createJSONStorage(() => localStorage),
    }
  )
);
