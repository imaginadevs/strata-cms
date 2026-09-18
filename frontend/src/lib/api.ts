import axios, { type AxiosError } from "axios";
import { useAuthStore } from "@/store/auth.store";
import type { ApiError, TokenResponse } from "@/types";

// All requests go to /api — in dev, Vite proxies to :8080; in prod, same origin.
export const api = axios.create({
  baseURL: "/api",
  headers: { "Content-Type": "application/json" },
  withCredentials: true,
});

// ── Request interceptor: attach the Bearer access token ───────────────────────
api.interceptors.request.use((config) => {
  const { accessToken } = useAuthStore.getState();
  if (accessToken) {
    config.headers.Authorization = `Bearer ${accessToken}`;
  }
  return config;
});

// ── Response interceptor: refresh the token on 401, then retry ────────────────
let refreshing = false;
let refreshQueue: Array<(token: string) => void> = [];

api.interceptors.response.use(
  (res) => res,
  async (error: AxiosError<ApiError>) => {
    const original = error.config!;
    const url = original.url ?? "";

    // A 401 from the auth endpoints themselves is a credentials/refresh failure,
    // not an expired session — reject so the caller (e.g. the Login form) can
    // show it. Never run refresh or hard-redirect for these.
    const isAuthEndpoint =
      url.includes("/auth/login") || url.includes("/auth/refresh");

    if (error.response?.status !== 401 || (original as any)._retry || isAuthEndpoint) {
      return Promise.reject(normaliseError(error));
    }

    const { refreshToken, clearSession } = useAuthStore.getState();
    if (!refreshToken) {
      clearSession();
      window.location.href = "/login";
      return Promise.reject(normaliseError(error));
    }

    // Queue concurrent 401s behind a single in-flight refresh.
    if (refreshing) {
      return new Promise((resolve) => {
        refreshQueue.push((token) => {
          original.headers!.Authorization = `Bearer ${token}`;
          resolve(api(original));
        });
      });
    }

    refreshing = true;
    (original as any)._retry = true;
    try {
      const { data } = await api.post<TokenResponse>("/auth/refresh", {
        refresh_token: refreshToken,
      });
      useAuthStore.getState().setSession(data);

      refreshQueue.forEach((cb) => cb(data.access_token));
      refreshQueue = [];

      original.headers!.Authorization = `Bearer ${data.access_token}`;
      return api(original);
    } catch {
      useAuthStore.getState().clearSession();
      window.location.href = "/login";
      return Promise.reject(normaliseError(error));
    } finally {
      refreshing = false;
    }
  }
);

// ── Typed fetch helpers ───────────────────────────────────────────────────────
export const get = <T>(url: string, params?: Record<string, unknown>) =>
  api.get<T>(url, { params }).then((r) => r.data);

export const post = <T>(url: string, data?: unknown) =>
  api.post<T>(url, data).then((r) => r.data);

export const patch = <T>(url: string, data?: unknown) =>
  api.patch<T>(url, data).then((r) => r.data);

export const put = <T>(url: string, data?: unknown) =>
  api.put<T>(url, data).then((r) => r.data);

export const del = <T>(url: string) =>
  api.delete<T>(url).then((r) => r.data);

// ── Error normalisation ───────────────────────────────────────────────────────
function normaliseError(err: AxiosError<ApiError>): Error & { status?: number } {
  const msg = err.response?.data?.error ?? err.message ?? "Unknown error";
  const out = new Error(msg) as Error & { status?: number };
  out.status = err.response?.status;
  return out;
}
