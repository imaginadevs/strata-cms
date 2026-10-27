/**
 * ContentOverview — the "home base" for content editors.
 *
 * Shows every (visible) collection as a card, grouped into sidebar folders, with
 * a quick search. A "Manage" mode lets admins create/rename/recolor/delete groups,
 * reorder groups and collections, and move collections between groups.
 */
import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Search, Database, Lock, Folder, Plus, Pencil, Trash2,
  ChevronUp, ChevronDown, Check, X, Settings2,
} from "lucide-react";
import { get, post, patch, del } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import type { Collection, CollectionGroup, PaginatedResponse } from "@/types";

const GROUP_COLORS = [
  "#6366f1", "#0ea5e9", "#10b981", "#f59e0b",
  "#ef4444", "#ec4899", "#8b5cf6", "#14b8a6",
];

// Best-effort item count for a collection card (silent on error).
function ItemCount({ name }: { name: string }) {
  const { t } = useTranslation();
  const { data } = useQuery({
    queryKey: ["item-count", name],
    queryFn: () => get<PaginatedResponse<unknown>>(`/items/${name}?limit=1`),
    staleTime: 30_000,
    retry: false,
  });
  if (data?.total == null) return null;
  return (
    <p className="text-xs text-slate-400">
      {t("content.itemCount", { count: data.total })}
    </p>
  );
}

function CollectionCard({
  c,
  manage,
  groups,
  onReassign,
  onMove,
  isFirst,
  isLast,
}: {
  c: Collection;
  manage: boolean;
  groups: CollectionGroup[];
  onReassign: (groupId: string | null) => void;
  onMove: (dir: -1 | 1) => void;
  isFirst: boolean;
  isLast: boolean;
}) {
  const { t } = useTranslation();
  const Icon = c.is_system ? Lock : Database;
  const color = c.color ?? undefined;

  const inner = (
    <>
      <span
        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
        style={color ? { backgroundColor: `${color}1e`, color } : undefined}
      >
        <Icon size={17} className={color ? undefined : "text-slate-400"} />
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-slate-800 dark:text-slate-100">
          {c.display_name}
        </p>
        {manage ? (
          <p className="truncate font-mono text-xs text-slate-400">{c.name}</p>
        ) : (
          <ItemCount name={c.name} />
        )}
      </div>
    </>
  );

  if (!manage) {
    return (
      <Link
        to="/content/$collection"
        params={{ collection: c.name }}
        className="flex items-center gap-3 rounded-xl border border-slate-200 bg-white p-3 transition hover:border-brand-300 hover:shadow-card dark:border-white/10 dark:bg-white/5 dark:hover:border-brand-500/40"
      >
        {inner}
      </Link>
    );
  }

  return (
    <div className="flex items-center gap-3 rounded-xl border border-slate-200 bg-white p-3 dark:border-white/10 dark:bg-white/5">
      {inner}
      <div className="flex shrink-0 items-center gap-1">
        <div className="flex flex-col">
          <button
            type="button"
            onClick={() => onMove(-1)}
            disabled={isFirst}
            className="text-slate-300 transition hover:text-slate-600 disabled:opacity-30 dark:hover:text-slate-200"
            title={t("content.moveUp")}
          >
            <ChevronUp size={14} />
          </button>
          <button
            type="button"
            onClick={() => onMove(1)}
            disabled={isLast}
            className="text-slate-300 transition hover:text-slate-600 disabled:opacity-30 dark:hover:text-slate-200"
            title={t("content.moveDown")}
          >
            <ChevronDown size={14} />
          </button>
        </div>
        <select
          value={c.group_id ?? ""}
          onChange={(e) => onReassign(e.target.value || null)}
          className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-xs text-slate-700 focus:border-brand-400 focus:outline-none dark:border-white/10 dark:bg-white/5 dark:text-slate-200"
          title={t("content.moveToGroup")}
        >
          <option value="">{t("content.ungrouped")}</option>
          {groups.map((g) => (
            <option key={g.id} value={g.id}>{g.name}</option>
          ))}
        </select>
      </div>
    </div>
  );
}

export default function ContentOverviewPage() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [manage, setManage] = useState(false);
  const [query, setQuery] = useState("");
  const [newGroup, setNewGroup] = useState("");
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState("");

  const { data: collections = [] } = useQuery({
    queryKey: ["collections"],
    queryFn: () => get<PaginatedResponse<Collection>>("/collections").then((r) => r.data ?? []),
  });
  const { data: groups = [] } = useQuery({
    queryKey: ["collection-groups"],
    queryFn: () => get<CollectionGroup[]>("/collection-groups"),
  });

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["collections"] });
    qc.invalidateQueries({ queryKey: ["collection-groups"] });
  };

  const reassign = useMutation({
    mutationFn: (v: { name: string; groupId: string | null }) =>
      patch(`/collections/${v.name}`, { group_id: v.groupId }),
    onSuccess: invalidate,
  });
  const reorderCols = useMutation({
    mutationFn: (items: { id: string; group_id: string | null; sort: number }[]) =>
      patch(`/collections/reorder`, { items }),
    onSuccess: invalidate,
  });
  const createGroup = useMutation({
    mutationFn: (b: { name: string; color: string; sort: number }) =>
      post<CollectionGroup>(`/collection-groups`, b),
    onSuccess: () => {
      setNewGroup("");
      invalidate();
    },
  });
  const updateGroup = useMutation({
    mutationFn: (v: { id: string; name?: string; color?: string }) =>
      patch(`/collection-groups/${v.id}`, { name: v.name, color: v.color }),
    onSuccess: () => {
      setEditingId(null);
      invalidate();
    },
  });
  const deleteGroup = useMutation({
    mutationFn: (id: string) => del(`/collection-groups/${id}`),
    onSuccess: invalidate,
  });
  const reorderGroups = useMutation({
    mutationFn: (order: string[]) => patch(`/collection-groups/reorder`, { order }),
    onSuccess: invalidate,
  });

  const q = query.trim().toLowerCase();
  const matches = (c: Collection) =>
    !q || c.display_name.toLowerCase().includes(q) || c.name.toLowerCase().includes(q);
  const byPos = (a: Collection, b: Collection) =>
    a.sort - b.sort || a.display_name.localeCompare(b.display_name);

  const visible = collections.filter((c) => !c.is_hidden);
  const sortedGroups = [...groups].sort((a, b) => a.sort - b.sort || a.name.localeCompare(b.name));
  const inGroup = (gid: string) => visible.filter((c) => c.group_id === gid && matches(c)).sort(byPos);
  const ungrouped = visible.filter((c) => !c.group_id && matches(c)).sort(byPos);

  // Move a collection within its ordered list and persist new sort indices.
  function moveWithin(list: Collection[], index: number, dir: -1 | 1, groupId: string | null) {
    const next = [...list];
    const target = index + dir;
    if (target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target], next[index]];
    reorderCols.mutate(next.map((c, i) => ({ id: c.id, group_id: groupId, sort: i })));
  }

  function moveGroup(index: number, dir: -1 | 1) {
    const next = [...sortedGroups];
    const target = index + dir;
    if (target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target], next[index]];
    reorderGroups.mutate(next.map((g) => g.id));
  }

  const renderCards = (list: Collection[], groupId: string | null) => (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {list.map((c, i) => (
        <CollectionCard
          key={c.id}
          c={c}
          manage={manage}
          groups={sortedGroups}
          onReassign={(gid) => reassign.mutate({ name: c.name, groupId: gid })}
          onMove={(dir) => moveWithin(list, i, dir, groupId)}
          isFirst={i === 0}
          isLast={i === list.length - 1}
        />
      ))}
    </div>
  );

  return (
    <div className="mx-auto max-w-5xl px-4 py-6 sm:px-6">
      {/* Header */}
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{t("content.title")}</h1>
          <p className="text-sm text-slate-400">
            {t("content.collectionCount", { count: visible.length })} ·{" "}
            {t("content.groupCount", { count: sortedGroups.length })}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 py-2 dark:border-white/10 dark:bg-white/5">
            <Search size={14} className="text-slate-400" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("content.searchCollections")}
              className="w-44 bg-transparent text-sm text-slate-700 placeholder:text-slate-400 focus:outline-none dark:text-slate-200"
            />
          </div>
          <Button variant={manage ? "primary" : "outline"} size="sm" onClick={() => setManage((m) => !m)}>
            <Settings2 size={14} className="mr-1.5" />
            {manage ? t("content.done") : t("content.manage")}
          </Button>
        </div>
      </div>

      {/* Manage: create group */}
      {manage && (
        <div className="mb-6 flex items-center gap-2 rounded-xl border border-dashed border-slate-300 bg-slate-50 p-3 dark:border-white/15 dark:bg-white/5">
          <Folder size={16} className="text-slate-400" />
          <input
            value={newGroup}
            onChange={(e) => setNewGroup(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && newGroup.trim()) {
                createGroup.mutate({ name: newGroup.trim(), color: GROUP_COLORS[groups.length % GROUP_COLORS.length], sort: groups.length });
              }
            }}
            placeholder={t("content.newGroupName")}
            className="flex-1 bg-transparent text-sm text-slate-700 placeholder:text-slate-400 focus:outline-none dark:text-slate-200"
          />
          <Button
            size="sm"
            onClick={() =>
              newGroup.trim() &&
              createGroup.mutate({ name: newGroup.trim(), color: GROUP_COLORS[groups.length % GROUP_COLORS.length], sort: groups.length })
            }
            disabled={!newGroup.trim() || createGroup.isPending}
          >
            <Plus size={14} className="mr-1" />
            {t("content.addGroup")}
          </Button>
        </div>
      )}

      {/* Groups */}
      <div className="space-y-8">
        {sortedGroups.map((g, gi) => {
          const items = inGroup(g.id);
          if (!manage && q && items.length === 0) return null;
          return (
            <section key={g.id}>
              <div className="mb-3 flex items-center gap-2">
                <span
                  className="flex h-6 w-6 items-center justify-center rounded-md"
                  style={g.color ? { backgroundColor: `${g.color}22`, color: g.color } : undefined}
                >
                  <Folder size={13} className={g.color ? undefined : "text-slate-400"} />
                </span>
                {editingId === g.id ? (
                  <div className="flex items-center gap-1.5">
                    <input
                      autoFocus
                      value={editName}
                      onChange={(e) => setEditName(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" && editName.trim()) updateGroup.mutate({ id: g.id, name: editName.trim() });
                        if (e.key === "Escape") setEditingId(null);
                      }}
                      className="rounded-lg border border-slate-200 bg-white px-2 py-1 text-sm text-slate-800 focus:border-brand-400 focus:outline-none dark:border-white/10 dark:bg-white/5 dark:text-slate-100"
                    />
                    <button type="button" onClick={() => editName.trim() && updateGroup.mutate({ id: g.id, name: editName.trim() })} className="text-emerald-500 hover:text-emerald-600" title={t("common.save")}>
                      <Check size={15} />
                    </button>
                    <button type="button" onClick={() => setEditingId(null)} className="text-slate-400 hover:text-slate-600" title={t("common.cancel")}>
                      <X size={15} />
                    </button>
                  </div>
                ) : (
                  <h2 className="text-sm font-semibold uppercase tracking-wide text-slate-600 dark:text-slate-300">
                    {g.name}
                  </h2>
                )}
                <span className="text-xs text-slate-400">{items.length}</span>

                {manage && editingId !== g.id && (
                  <div className="ml-auto flex items-center gap-1">
                    {GROUP_COLORS.slice(0, 6).map((col) => (
                      <button
                        key={col}
                        type="button"
                        onClick={() => updateGroup.mutate({ id: g.id, color: col })}
                        className="h-4 w-4 rounded-full ring-1 ring-white/60 transition hover:scale-110"
                        style={{ backgroundColor: col }}
                        title={t("content.setColor")}
                        aria-label={t("content.setGroupColor", { color: col })}
                      />
                    ))}
                    <button type="button" onClick={() => moveGroup(gi, -1)} disabled={gi === 0} className="ml-1 text-slate-300 transition hover:text-slate-600 disabled:opacity-30 dark:hover:text-slate-200" title={t("content.moveGroupUp")}>
                      <ChevronUp size={15} />
                    </button>
                    <button type="button" onClick={() => moveGroup(gi, 1)} disabled={gi === sortedGroups.length - 1} className="text-slate-300 transition hover:text-slate-600 disabled:opacity-30 dark:hover:text-slate-200" title={t("content.moveGroupDown")}>
                      <ChevronDown size={15} />
                    </button>
                    <button type="button" onClick={() => { setEditingId(g.id); setEditName(g.name); }} className="text-slate-300 transition hover:text-brand-500" title={t("content.rename")}>
                      <Pencil size={14} />
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        if (confirm(t("content.deleteGroupConfirm", { name: g.name }))) deleteGroup.mutate(g.id);
                      }}
                      className="text-slate-300 transition hover:text-red-500"
                      title={t("content.deleteGroup")}
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                )}
              </div>
              {items.length > 0 ? (
                renderCards(items, g.id)
              ) : (
                <p className="rounded-xl border border-dashed border-slate-200 px-4 py-6 text-center text-sm text-slate-400 dark:border-white/10">
                  {manage ? t("content.noCollectionsManage") : t("content.noCollections")}
                </p>
              )}
            </section>
          );
        })}

        {/* Ungrouped */}
        {(ungrouped.length > 0 || sortedGroups.length === 0) && (
          <section>
            {sortedGroups.length > 0 && (
              <div className="mb-3 flex items-center gap-2">
                <h2 className="text-sm font-semibold uppercase tracking-wide text-slate-500">{t("content.ungrouped")}</h2>
                <span className="text-xs text-slate-400">{ungrouped.length}</span>
              </div>
            )}
            {ungrouped.length > 0 ? (
              renderCards(ungrouped, null)
            ) : (
              <p className="text-sm text-slate-400">{t("content.noUngrouped")}</p>
            )}
          </section>
        )}
      </div>
    </div>
  );
}
