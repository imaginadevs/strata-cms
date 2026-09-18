import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import type { AuthUser, TokenResponse } from "@/types";

interface AuthState {
  isAuthenticated: boolean;
  accessToken: string | null;
  refreshToken: string | null;
  expiresAt: string | null;
  user: AuthUser | null;

  setSession: (response: TokenResponse) => void;
  clearSession: () => void;
  updateUser: (patch: Partial<AuthUser>) => void;
}

/**
 * Client-held JWT session. The token is persisted to localStorage so it survives
 * reloads, sent as `Authorization: Bearer <accessToken>` by the api/graphql
 * layers, and passed to SSE via the query string. Uniform across all consumers.
 */
export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      isAuthenticated: false,
      accessToken: null,
      refreshToken: null,
      expiresAt: null,
      user: null,

      setSession: (response) =>
        set({
          isAuthenticated: true,
          accessToken: response.access_token,
          refreshToken: response.refresh_token,
          expiresAt: response.expires_at,
          user: response.user,
        }),

      clearSession: () =>
        set({
          isAuthenticated: false,
          accessToken: null,
          refreshToken: null,
          expiresAt: null,
          user: null,
        }),

      updateUser: (patch) =>
        set((state) => (state.user ? { user: { ...state.user, ...patch } } : state)),
    }),
    {
      name: "cms-auth",
      storage: createJSONStorage(() => localStorage),
    }
  )
);
