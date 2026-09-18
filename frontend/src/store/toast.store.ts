import { create } from "zustand";
import type { NotificationType } from "@/types";

export interface ToastItem {
  id: string;
  title: string;
  description?: string;
  variant?: NotificationType;
  link?: string | null;
  duration?: number;
}

interface ToastState {
  toasts: ToastItem[];
  push: (toast: Omit<ToastItem, "id"> & { id?: string }) => string;
  dismiss: (id: string) => void;
}

export const useToastStore = create<ToastState>((set) => ({
  toasts: [],
  push: (toast) => {
    const id = toast.id ?? crypto.randomUUID();
    set((s) => ({
      toasts: [...s.toasts, { duration: 4_500, ...toast, id }],
    }));
    return id;
  },
  dismiss: (id) =>
    set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
}));

export function toast(input: Omit<ToastItem, "id"> & { id?: string }) {
  return useToastStore.getState().push(input);
}
