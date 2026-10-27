import { Database, Users, ScrollText, Activity } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { get } from "@/lib/api";
import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { duration, easing } from "@/lib/motion";
import type { Collection, User, AuditLog, PaginatedResponse } from "@/types";

const stats = [
  { labelKey: "dashboard.collections", icon: Database,   gradient: "from-blue-500 to-indigo-500",     key: "collections" },
  { labelKey: "dashboard.users",       icon: Users,      gradient: "from-emerald-500 to-teal-500",    key: "users" },
  { labelKey: "dashboard.auditLogs",   icon: ScrollText, gradient: "from-amber-500 to-orange-500",    key: "audit" },
  { labelKey: "dashboard.activeNow",   icon: Activity,   gradient: "from-fuchsia-500 to-purple-500",  key: "active" },
] as const;

export default function DashboardPage() {
  const { t } = useTranslation();
  // Use the same queryFn shape as Collections.tsx so the shared cache is always Collection[].
  const { data: collections = [] } = useQuery({
    queryKey: ["collections"],
    queryFn: () =>
      get<PaginatedResponse<Collection>>("/collections").then((r) => r.data ?? []),
  });

  const { data: users } = useQuery({
    queryKey: ["users"],
    queryFn: () => get<PaginatedResponse<User>>("/users"),
  });

  const { data: audit } = useQuery({
    queryKey: ["audit", "recent"],
    queryFn: () => get<PaginatedResponse<AuditLog>>("/audit", { limit: 10 }),
  });

  const counts: Record<string, number> = {
    collections: collections.length,
    users: users?.total ?? 0,
    audit: audit?.data?.length ?? 0,
    active: 1,
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">{t("dashboard.title")}</h1>
        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
          {t("dashboard.subtitle")}
        </p>
      </div>

      {/* Stat cards */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        {stats.map(({ labelKey, icon: Icon, gradient, key }, i) => (
          <Card
            key={key}
            interactive
            className="flex items-center gap-4"
            transition={{ duration: duration.base, ease: easing.out, delay: i * 0.06 }}
          >
            <div
              className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-xl
                          bg-gradient-to-br ${gradient} text-white shadow-sm`}
            >
              <Icon size={20} />
            </div>
            <div>
              <p className="text-2xl font-bold text-slate-900 dark:text-white">{counts[key]}</p>
              <p className="text-xs text-slate-500 dark:text-slate-400">{t(labelKey)}</p>
            </div>
          </Card>
        ))}
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        {/* Recent collections */}
        <Card transition={{ duration: duration.base, ease: easing.out, delay: 0.24 }}>
          <CardHeader>
            <CardTitle>{t("dashboard.collections")}</CardTitle>
          </CardHeader>
          {collections.slice(0, 5).map((c) => (
            <div
              key={c.id}
              className="flex items-center gap-3 border-b border-slate-100 py-2.5 last:border-0 dark:border-white/5"
            >
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-brand-50 dark:bg-brand-500/15">
                <Database size={14} className="text-brand-600 dark:text-brand-300" />
              </div>
              <div>
                <p className="text-sm font-medium text-slate-800 dark:text-slate-200">{c.display_name}</p>
                <p className="text-xs text-slate-400">{c.name}</p>
              </div>
              <span className={`badge ml-auto ${c.is_system ? "badge-gray" : "badge-blue"}`}>
                {c.is_system ? t("dashboard.system") : c.type}
              </span>
            </div>
          ))}
          {!collections.length && (
            <p className="py-4 text-center text-sm text-slate-400">{t("dashboard.noCollections")}</p>
          )}
        </Card>

        {/* Recent audit */}
        <Card transition={{ duration: duration.base, ease: easing.out, delay: 0.3 }}>
          <CardHeader>
            <CardTitle>{t("dashboard.recentActivity")}</CardTitle>
          </CardHeader>
          {audit?.data?.slice(0, 6).map((log) => (
            <div
              key={log.id}
              className="flex items-start gap-3 border-b border-slate-100 py-2.5 last:border-0 dark:border-white/5"
            >
              <span className={`badge mt-0.5 shrink-0 ${
                log.operation === "create" ? "badge-green"
                : log.operation === "delete" ? "badge-red"
                : log.operation === "login" ? "badge-blue"
                : "badge-amber"
              }`}>
                {t(`audit.op.${log.operation}`, { defaultValue: log.operation })}
              </span>
              <div className="min-w-0">
                <p className="truncate text-sm text-slate-700 dark:text-slate-300">
                  {log.collection}
                  {log.item_id ? ` #${log.item_id.slice(0, 8)}` : ""}
                </p>
                <p className="text-xs text-slate-400">{log.user_email ?? t("dashboard.system")}</p>
              </div>
            </div>
          ))}
          {!audit?.data?.length && (
            <p className="py-4 text-center text-sm text-slate-400">{t("dashboard.noActivity")}</p>
          )}
        </Card>
      </div>
    </div>
  );
}
