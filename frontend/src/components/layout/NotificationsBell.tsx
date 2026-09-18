import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import * as Popover from "@radix-ui/react-popover";
import { formatDistanceToNow } from "date-fns";
import {
  AlertCircle,
  AlertTriangle,
  Bell,
  CheckCheck,
  CheckCircle2,
  Info,
} from "lucide-react";
import { useNavigate } from "@tanstack/react-router";
import { get, post } from "@/lib/api";
import { cn } from "@/lib/utils";
import { notificationKeys } from "@/hooks/useNotificationsRealtime";
import type { Notification, NotificationType, PaginatedResponse } from "@/types";

const TYPE_ICON: Record<
  NotificationType,
  { icon: typeof Info; className: string }
> = {
  info: {
    icon: Info,
    className: "bg-sky-50 text-sky-600 dark:bg-sky-500/15 dark:text-sky-300",
  },
  success: {
    icon: CheckCircle2,
    className:
      "bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-300",
  },
  warning: {
    icon: AlertTriangle,
    className:
      "bg-amber-50 text-amber-600 dark:bg-amber-500/15 dark:text-amber-300",
  },
  error: {
    icon: AlertCircle,
    className: "bg-red-50 text-red-600 dark:bg-red-500/15 dark:text-red-300",
  },
};

function relativeTime(iso: string) {
  try {
    return formatDistanceToNow(new Date(iso), { addSuffix: true });
  } catch {
    return "";
  }
}

export function NotificationsBell() {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const { data: countData } = useQuery({
    queryKey: notificationKeys.unreadCount(),
    queryFn: () => get<{ count: number }>("/notifications/unread-count"),
    refetchInterval: 60_000,
  });
  const unread = countData?.count ?? 0;

  const { data: listData, isLoading, isFetching } = useQuery({
    queryKey: notificationKeys.list(),
    queryFn: () =>
      get<PaginatedResponse<Notification>>("/notifications", {
        page: 1,
        limit: 20,
      }),
    enabled: open,
  });
  const items = listData?.data ?? [];

  const markReadMut = useMutation({
    mutationFn: (id: string) => post(`/notifications/${id}/read`),
    onMutate: async (id) => {
      await queryClient.cancelQueries({ queryKey: notificationKeys.all });
      const prevCount = queryClient.getQueryData<{ count: number }>(
        notificationKeys.unreadCount(),
      );
      const prevList = queryClient.getQueryData<PaginatedResponse<Notification>>(
        notificationKeys.list(),
      );
      const target = prevList?.data.find((n) => n.id === id);
      const wasUnread = target ? !target.read_at : true;

      if (wasUnread) {
        queryClient.setQueryData(notificationKeys.unreadCount(), {
          count: Math.max(0, (prevCount?.count ?? 1) - 1),
        });
      }
      if (prevList) {
        const now = new Date().toISOString();
        queryClient.setQueryData(notificationKeys.list(), {
          ...prevList,
          data: prevList.data.map((n) =>
            n.id === id && !n.read_at ? { ...n, read_at: now } : n,
          ),
        });
      }
      return { prevCount, prevList };
    },
    onError: (_err, _id, ctx) => {
      if (ctx?.prevCount) {
        queryClient.setQueryData(notificationKeys.unreadCount(), ctx.prevCount);
      }
      if (ctx?.prevList) {
        queryClient.setQueryData(notificationKeys.list(), ctx.prevList);
      }
    },
  });

  const markAllMut = useMutation({
    mutationFn: () => post("/notifications/read-all"),
    onMutate: async () => {
      await queryClient.cancelQueries({ queryKey: notificationKeys.all });
      const prevCount = queryClient.getQueryData<{ count: number }>(
        notificationKeys.unreadCount(),
      );
      const prevList = queryClient.getQueryData<PaginatedResponse<Notification>>(
        notificationKeys.list(),
      );
      queryClient.setQueryData(notificationKeys.unreadCount(), { count: 0 });
      if (prevList) {
        const now = new Date().toISOString();
        queryClient.setQueryData(notificationKeys.list(), {
          ...prevList,
          data: prevList.data.map((n) =>
            n.read_at ? n : { ...n, read_at: now },
          ),
        });
      }
      return { prevCount, prevList };
    },
    onError: (_err, _v, ctx) => {
      if (ctx?.prevCount) {
        queryClient.setQueryData(notificationKeys.unreadCount(), ctx.prevCount);
      }
      if (ctx?.prevList) {
        queryClient.setQueryData(notificationKeys.list(), ctx.prevList);
      }
    },
  });

  function handleItemClick(n: Notification) {
    if (!n.read_at) markReadMut.mutate(n.id);
    setOpen(false);
    if (n.link?.startsWith("/")) {
      navigate({ to: n.link });
    }
  }

  const badge =
    unread > 99 ? "99+" : unread > 0 ? String(unread) : null;

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>
        <button
          type="button"
          aria-label={
            unread > 0
              ? `Notifications, ${unread} unread`
              : "Notifications"
          }
          className="relative flex h-9 w-9 items-center justify-center rounded-xl
                     text-slate-500 transition hover:bg-slate-100
                     dark:text-slate-300 dark:hover:bg-white/10"
        >
          <Bell size={17} strokeWidth={1.8} />
          {badge && (
            <span
              className="absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center
                         rounded-full bg-red-500 px-1 text-[10px] font-bold leading-none text-white
                         shadow-sm ring-2 ring-white dark:ring-slate-900"
            >
              {badge}
            </span>
          )}
        </button>
      </Popover.Trigger>

      <Popover.Portal>
        <Popover.Content
          align="end"
          sideOffset={8}
          className="z-50 w-[min(100vw-1.5rem,22rem)] origin-top-right animate-slide-up
                     overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-lg
                     dark:border-white/10 dark:bg-slate-900"
        >
          <div className="flex items-center justify-between border-b border-slate-100 px-4 py-3 dark:border-white/10">
            <div>
              <p className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                Notifications
              </p>
              {unread > 0 && (
                <p className="text-[11px] text-slate-400">
                  {unread} unread
                </p>
              )}
            </div>
            <button
              type="button"
              disabled={unread === 0 || markAllMut.isPending}
              onClick={() => markAllMut.mutate()}
              className="inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-xs font-medium
                         text-brand-600 transition hover:bg-brand-50 disabled:pointer-events-none
                         disabled:opacity-40 dark:text-brand-300 dark:hover:bg-brand-500/10"
            >
              <CheckCheck size={13} />
              Mark all read
            </button>
          </div>

          <div className="max-h-[min(24rem,60vh)] overflow-y-auto">
            {isLoading && items.length === 0 ? (
              <p className="px-4 py-10 text-center text-sm text-slate-400">
                Loading…
              </p>
            ) : items.length === 0 ? (
              <p className="px-4 py-10 text-center text-sm text-slate-400">
                You&apos;re all caught up
              </p>
            ) : (
              <ul className="divide-y divide-slate-50 dark:divide-white/5">
                {items.map((n) => {
                  const style = TYPE_ICON[n.type] ?? TYPE_ICON.info;
                  const Icon = style.icon;
                  const unreadItem = !n.read_at;
                  return (
                    <li key={n.id}>
                      <button
                        type="button"
                        onClick={() => handleItemClick(n)}
                        className={cn(
                          "flex w-full gap-3 px-4 py-3 text-left transition",
                          "hover:bg-slate-50/80 dark:hover:bg-white/[0.04]",
                          unreadItem && "bg-brand-50/40 dark:bg-brand-500/[0.07]",
                        )}
                      >
                        <div
                          className={cn(
                            "mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg",
                            style.className,
                          )}
                        >
                          <Icon size={14} />
                        </div>
                        <div className="min-w-0 flex-1">
                          <div className="flex items-start gap-2">
                            <p
                              className={cn(
                                "flex-1 text-sm leading-snug text-slate-800 dark:text-slate-100",
                                unreadItem && "font-semibold",
                              )}
                            >
                              {n.title}
                            </p>
                            {unreadItem && (
                              <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-brand-500" />
                            )}
                          </div>
                          {n.body ? (
                            <p className="mt-0.5 line-clamp-2 text-xs text-slate-500 dark:text-slate-400">
                              {n.body}
                            </p>
                          ) : null}
                          <p className="mt-1 text-[10px] text-slate-400">
                            {relativeTime(n.created_at)}
                          </p>
                        </div>
                      </button>
                    </li>
                  );
                })}
              </ul>
            )}
            {isFetching && !isLoading && (
              <p className="border-t border-slate-50 px-4 py-2 text-center text-[10px] text-slate-400 dark:border-white/5">
                Updating…
              </p>
            )}
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
