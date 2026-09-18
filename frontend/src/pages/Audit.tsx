import { useQuery } from "@tanstack/react-query";
import { get } from "@/lib/api";
import { Card } from "@/components/ui/Card";
import { formatDate } from "@/lib/utils";
import type { PaginatedResponse, AuditLog } from "@/types";

const OPERATION_STYLE: Record<string, string> = {
  create: "badge-green",
  update: "badge-amber",
  delete: "badge-red",
  login:  "badge-blue",
  logout: "badge-gray",
};

export default function AuditPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["audit"],
    queryFn: () => get<PaginatedResponse<AuditLog>>("/audit"),
  });

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">Audit Log</h1>
        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">All changes across your CMS</p>
      </div>

      <Card className="p-0 overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-slate-100 dark:border-white/10 text-left text-xs font-medium text-slate-500 dark:text-slate-400">
              <th className="px-5 py-3">Operation</th>
              <th className="px-5 py-3">Collection</th>
              <th className="px-5 py-3">User</th>
              <th className="px-5 py-3">Changed fields</th>
              <th className="px-5 py-3">Date</th>
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr>
                <td colSpan={5} className="py-12 text-center text-slate-400">Loading…</td>
              </tr>
            )}
            {data?.data?.map((log) => (
              <tr key={log.id} className="border-b border-slate-100 dark:border-white/10 last:border-0 hover:bg-slate-50/60 dark:hover:bg-white/5">
                <td className="px-5 py-3">
                  <span className={`badge ${OPERATION_STYLE[log.operation] ?? "badge-gray"}`}>
                    {log.operation}
                  </span>
                </td>
                <td className="px-5 py-3 font-mono text-xs text-slate-600 dark:text-slate-300">
                  {log.collection}
                  {log.item_id ? `/${log.item_id.slice(0, 8)}` : ""}
                </td>
                <td className="px-5 py-3 text-slate-600 dark:text-slate-300">{log.user_email ?? "system"}</td>
                <td className="px-5 py-3 text-xs text-slate-400">
                  {log.changed_fields?.join(", ") ?? "—"}
                </td>
                <td className="px-5 py-3 text-slate-400">{formatDate(log.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </div>
  );
}
