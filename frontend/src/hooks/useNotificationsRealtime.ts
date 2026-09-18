import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { sseClient } from "@/lib/sse";
import { toast } from "@/store/toast.store";
import type {
  Notification,
  NotificationCreatedPayload,
  NotificationReadPayload,
  PaginatedResponse,
  SSEEvent,
} from "@/types";

export const notificationKeys = {
  all: ["notifications"] as const,
  list: (params?: { unread?: boolean }) =>
    [...notificationKeys.all, "list", params ?? {}] as const,
  unreadCount: () => [...notificationKeys.all, "unread-count"] as const,
};

function payloadToNotification(p: NotificationCreatedPayload): Notification {
  return {
    id: p.id,
    recipient_id: p.recipient_id,
    type: p.type ?? "info",
    title: p.title,
    body: p.body ?? "",
    link: p.link ?? null,
    meta: p.meta,
    read_at: null,
    created_at: p.created_at,
  };
}

/**
 * Keeps inbox React Query caches in sync with SSE, and surfaces toasts.
 * Mount once while authenticated (AppShell).
 */
export function useNotificationsRealtime() {
  const queryClient = useQueryClient();

  useEffect(() => {
    const onCreated = (ev: SSEEvent<NotificationCreatedPayload>) => {
      const payload = ev.payload;
      if (!payload?.id) return;

      const item = payloadToNotification(payload);

      queryClient.setQueryData<{ count: number }>(
        notificationKeys.unreadCount(),
        (prev) => ({ count: (prev?.count ?? 0) + 1 }),
      );

      queryClient.setQueriesData<PaginatedResponse<Notification>>(
        { queryKey: [...notificationKeys.all, "list"] },
        (prev) => {
          if (!prev) return prev;
          if (prev.data.some((n) => n.id === item.id)) return prev;
          return {
            ...prev,
            data: [item, ...prev.data],
            total: prev.total + 1,
          };
        },
      );

      toast({
        title: item.title,
        description: item.body || undefined,
        variant: item.type,
        link: item.link,
      });
    };

    const onRead = (ev: SSEEvent<NotificationReadPayload>) => {
      const payload = ev.payload;
      if (!payload) return;

      if (payload.all) {
        queryClient.setQueryData(notificationKeys.unreadCount(), { count: 0 });
        queryClient.setQueriesData<PaginatedResponse<Notification>>(
          { queryKey: [...notificationKeys.all, "list"] },
          (prev) => {
            if (!prev) return prev;
            const now = new Date().toISOString();
            return {
              ...prev,
              data: prev.data.map((n) =>
                n.read_at ? n : { ...n, read_at: now },
              ),
            };
          },
        );
        return;
      }

      if (payload.id) {
        const id = payload.id;
        queryClient.setQueryData<{ count: number }>(
          notificationKeys.unreadCount(),
          (prev) => ({ count: Math.max(0, (prev?.count ?? 1) - 1) }),
        );
        queryClient.setQueriesData<PaginatedResponse<Notification>>(
          { queryKey: [...notificationKeys.all, "list"] },
          (prev) => {
            if (!prev) return prev;
            const now = new Date().toISOString();
            return {
              ...prev,
              data: prev.data.map((n) =>
                n.id === id && !n.read_at ? { ...n, read_at: now } : n,
              ),
            };
          },
        );
      }
    };

    const offCreated = sseClient.on("notification.created", onCreated);
    const offRead = sseClient.on("notification.read", onRead);
    return () => {
      offCreated();
      offRead();
    };
  }, [queryClient]);
}
