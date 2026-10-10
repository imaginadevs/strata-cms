/**
 * fields.tsx — shared dynamic field renderers for collection items.
 *
 * Extracted from Content.tsx so the same inputs drive both the top-level item
 * form and nested sub-item (relation) rows. Keep these presentation-only and
 * schema-driven — no collection-specific logic.
 */
import { useState, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { motion, AnimatePresence } from "framer-motion";
import * as Select from "@radix-ui/react-select";
import * as Switch from "@radix-ui/react-switch";
import * as Popover from "@radix-ui/react-popover";
import {
  X, ChevronDown, EyeOff, RefreshCw, Clock, Check, Globe,
  Upload, File as FileIcon, ExternalLink,
} from "lucide-react";
import { get, upload } from "@/lib/api";
import { RichTextEditor } from "@/components/ui/RichTextEditor";
import type { Field, PaginatedResponse, Tenant, TenantSettings } from "@/types";

// ── Language helpers ──────────────────────────────────────────────────────────

export function useAvailableLanguages(): string[] {
  const { data } = useQuery({
    queryKey: ["settings"],
    queryFn: () =>
      get<{ tenant: Tenant; settings: TenantSettings }>("/settings"),
    staleTime: 60_000,
  });
  const langs = data?.settings.languages;
  return langs && langs.length > 0 ? langs : ["en"];
}

// Small language selector used in both the create page and the edit panel.
export function LanguageSelector({
  value,
  onChange,
  languages,
}: {
  value: string;
  onChange: (lang: string) => void;
  languages: string[];
}) {
  return (
    <div className="flex items-center gap-2 rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-1.5">
      <Globe size={13} className="shrink-0 text-slate-400" />
      <span className="text-xs font-medium text-slate-500 dark:text-slate-400">Language</span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="appearance-none bg-transparent text-xs font-semibold text-slate-700 dark:text-slate-200 focus:outline-none"
      >
        {languages.map((lang) => (
          <option key={lang} value={lang}>{lang.toUpperCase()}</option>
        ))}
      </select>
    </div>
  );
}

// ── JsonInput sub-component ───────────────────────────────────────────────────

export function JsonInput({
  value,
  onChange,
  error,
}: {
  value: unknown;
  onChange: (v: unknown) => void;
  error?: string;
}) {
  const base =
    "w-full rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-2 text-sm text-slate-800 dark:text-slate-100 " +
    "placeholder:text-slate-400 focus:border-brand-400 dark:focus:border-brand-500/60 focus:outline-none focus:ring-2 focus:ring-brand-100 dark:focus:ring-brand-500/20" +
    (error ? " border-red-300 focus:border-red-400 focus:ring-red-100" : "");

  const helperBtn =
    "shrink-0 rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-2 text-xs font-medium text-slate-600 dark:text-slate-300 transition hover:bg-slate-50 dark:hover:bg-white/5 hover:text-slate-800 dark:hover:text-slate-100";

  const [localText, setLocalText] = useState(
    value == null ? "" : typeof value === "string" ? value : JSON.stringify(value, null, 2)
  );
  const [feedback, setFeedback] = useState<{ ok: boolean; msg: string } | null>(null);

  function showFeedback(ok: boolean, msg: string) {
    setFeedback({ ok, msg });
    setTimeout(() => setFeedback(null), 2000);
  }

  function handleValidate() {
    try {
      JSON.parse(localText);
      showFeedback(true, "✓ Valid JSON");
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      showFeedback(false, msg);
    }
  }

  function handleFormat() {
    try {
      const formatted = JSON.stringify(JSON.parse(localText), null, 2);
      setLocalText(formatted);
      onChange(formatted);
      showFeedback(true, "✓ Formatted");
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      showFeedback(false, msg);
    }
  }

  return (
    <div className="space-y-1.5">
      <textarea
        className={base + " min-h-[120px] resize-y font-mono text-xs"}
        value={localText}
        onChange={(e) => {
          setLocalText(e.target.value);
          onChange(e.target.value);
        }}
        placeholder="{}"
      />
      <div className="flex items-center gap-2">
        <button type="button" className={helperBtn} onClick={handleValidate}>
          <Check size={11} className="mr-1 inline" />
          Validate
        </button>
        <button type="button" className={helperBtn} onClick={handleFormat}>
          <RefreshCw size={11} className="mr-1 inline" />
          Format
        </button>
        {feedback && (
          <span
            className={`text-xs ${feedback.ok ? "text-green-600 dark:text-emerald-300" : "text-red-500 dark:text-red-400"}`}
          >
            {feedback.msg}
          </span>
        )}
      </div>
    </div>
  );
}

// ── RelationInput sub-component ───────────────────────────────────────────────

export function RelationInput({
  field,
  value,
  onChange,
  error,
}: {
  field: Field;
  value: unknown;
  onChange: (v: unknown) => void;
  error?: string;
}) {
  const base =
    "w-full rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-2 text-sm text-slate-800 dark:text-slate-100 " +
    "placeholder:text-slate-400 focus:border-brand-400 dark:focus:border-brand-500/60 focus:outline-none focus:ring-2 focus:ring-brand-100 dark:focus:ring-brand-500/20" +
    (error ? " border-red-300 focus:border-red-400 focus:ring-red-100" : "");

  const relatedCollection = (field.options as Record<string, unknown>)
    ?.related_collection as string | undefined;
  // display_field is configured when the relation is created; it names the column
  // of the related collection to render as the label instead of the raw UUID.
  const displayField = (field.options as Record<string, unknown>)
    ?.display_field as string | undefined;

  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");

  const { data: relatedItems } = useQuery({
    queryKey: ["relation-items", relatedCollection],
    queryFn: () =>
      get<PaginatedResponse<Record<string, unknown>>>(
        `/items/${relatedCollection}?limit=100`
      ),
    enabled: !!relatedCollection && open,
  });

  // Resolve the currently-selected item's label up front so the field shows a
  // readable value immediately, without the user having to open the picker.
  const { data: selectedItem } = useQuery({
    queryKey: ["relation-item", relatedCollection, value],
    queryFn: () =>
      get<Record<string, unknown>>(`/items/${relatedCollection}/${value}`),
    enabled: !!relatedCollection && value != null && value !== "",
  });

  if (!relatedCollection) {
    return (
      <div className="space-y-1">
        <input
          type="text"
          className={base + " font-mono"}
          value={value == null ? "" : String(value)}
          onChange={(e) => onChange(e.target.value || null)}
          placeholder="Enter UUID…"
        />
        <p className="text-xs text-slate-400">Related collection not configured</p>
      </div>
    );
  }

  const systemKeys = new Set(["id", "tenant_id", "created_at", "updated_at", "created_by", "updated_by"]);

  function getDisplayValue(item: Record<string, unknown>): string {
    // Prefer the configured display field when present and non-empty.
    if (displayField) {
      const v = item[displayField];
      if (typeof v === "string" && v.length > 0) return v;
      if (typeof v === "number") return String(v);
    }
    // Fall back to the first human-readable field (legacy relations with no display_field).
    for (const [k, v] of Object.entries(item)) {
      if (systemKeys.has(k)) continue;
      if (typeof v === "string" && v.length > 0) return v;
      if (typeof v === "number") return String(v);
    }
    const id = String(item.id ?? "");
    return id.length > 8 ? id.slice(0, 8) + "…" : id;
  }

  const filtered = (relatedItems?.data ?? []).filter((item) => {
    if (!search) return true;
    const display = getDisplayValue(item).toLowerCase();
    return display.includes(search.toLowerCase()) || String(item.id).includes(search);
  });

  const currentDisplay = value
    ? (() => {
        const found = (relatedItems?.data ?? []).find((i) => i.id === value);
        if (found) return getDisplayValue(found);
        if (selectedItem) return getDisplayValue(selectedItem);
        return String(value);
      })()
    : null;

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <div className="flex items-center gap-2">
        <div className="flex-1 rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-2 text-sm">
          {currentDisplay ? (
            <span className="text-slate-800 dark:text-slate-100">{currentDisplay}</span>
          ) : (
            <span className="text-slate-400">No selection</span>
          )}
        </div>
        <Popover.Trigger asChild>
          <button
            type="button"
            className="shrink-0 rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-2 text-xs font-medium text-slate-600 dark:text-slate-300 transition hover:bg-slate-50 dark:hover:bg-white/5 hover:text-slate-800 dark:hover:text-slate-100"
          >
            Browse
          </button>
        </Popover.Trigger>
        {value != null && (
          <button
            type="button"
            onClick={() => onChange(null)}
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 text-slate-400 transition hover:bg-red-50 dark:hover:bg-red-500/10 hover:text-red-500 dark:hover:text-red-400"
          >
            <X size={13} />
          </button>
        )}
      </div>
      <Popover.Portal>
        <Popover.Content
          className="z-50 w-72 rounded-xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-800 p-2 shadow-lg"
          sideOffset={4}
        >
          <input
            autoFocus
            type="text"
            className="mb-2 w-full rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-1.5 text-sm text-slate-800 dark:text-slate-100 placeholder:text-slate-400 focus:border-brand-400 dark:focus:border-brand-500/60 focus:outline-none focus:ring-2 focus:ring-brand-100 dark:focus:ring-brand-500/20"
            placeholder="Search…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <div className="max-h-56 overflow-y-auto">
            {filtered.length === 0 ? (
              <p className="px-3 py-2 text-xs text-slate-400">No items found</p>
            ) : (
              filtered.map((item) => (
                <button
                  key={String(item.id)}
                  type="button"
                  className="w-full rounded-lg px-3 py-2 text-left text-sm text-slate-700 dark:text-slate-200 transition hover:bg-brand-50 dark:hover:bg-brand-500/15 hover:text-brand-700 dark:hover:text-brand-300"
                  onClick={() => {
                    onChange(item.id);
                    setOpen(false);
                    setSearch("");
                  }}
                >
                  {getDisplayValue(item)}
                  <span className="ml-2 text-xs text-slate-400">
                    {String(item.id).slice(0, 8)}…
                  </span>
                </button>
              ))
            )}
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

// ── File / image inputs ───────────────────────────────────────────────────────

/** Metadata returned by GET/POST /files — `url` is presigned (private) or CDN (public). */
interface StoredFile {
  id: string;
  filename: string;
  content_type: string;
  size: number;
  is_public: boolean;
  url: string;
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

/** A single stored file, resolved by id, shown as an image thumbnail or a file chip. */
function FileCard({ id, onRemove }: { id: string; onRemove: () => void }) {
  const { data: file, isLoading } = useQuery({
    queryKey: ["file", id],
    queryFn: () => get<StoredFile>(`/files/${id}`),
    enabled: !!id,
  });

  const isImage = file?.content_type?.startsWith("image/");

  return (
    <div className="flex items-center gap-3 rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 p-2">
      <div className="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-md bg-slate-100 dark:bg-white/10">
        {isImage && file?.url ? (
          <img src={file.url} alt={file.filename} className="h-full w-full object-cover" />
        ) : (
          <FileIcon size={18} className="text-slate-400" />
        )}
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm text-slate-800 dark:text-slate-100">
          {isLoading ? "Loading…" : file?.filename ?? id}
        </p>
        {file && (
          <p className="text-xs text-slate-400">
            {formatBytes(file.size)}
            {file.is_public ? " · public" : " · private"}
          </p>
        )}
      </div>
      {file?.url && (
        <a
          href={file.url}
          target="_blank"
          rel="noreferrer"
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-slate-200 dark:border-white/10 text-slate-400 transition hover:text-slate-600 dark:hover:text-slate-200"
          title="Open"
        >
          <ExternalLink size={13} />
        </a>
      )}
      <button
        type="button"
        onClick={onRemove}
        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-slate-200 dark:border-white/10 text-slate-400 transition hover:bg-red-50 dark:hover:bg-red-500/10 hover:text-red-500 dark:hover:text-red-400"
        title="Remove"
      >
        <X size={13} />
      </button>
    </div>
  );
}

const uploadBtn =
  "flex items-center gap-2 rounded-lg border border-dashed border-slate-300 dark:border-white/15 bg-white dark:bg-white/5 px-3 py-2 text-sm font-medium text-slate-600 dark:text-slate-300 transition hover:border-brand-400 hover:text-brand-600 dark:hover:text-brand-300 disabled:cursor-not-allowed disabled:opacity-60";

/** Hidden <input type=file> + a trigger button; uploads then calls onUploaded(id). */
function useFileUpload(onUploaded: (id: string) => void, multiple: boolean) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const [uploadErr, setUploadErr] = useState<string | null>(null);

  async function handleFiles(list: FileList | null) {
    if (!list || list.length === 0) return;
    setUploading(true);
    setUploadErr(null);
    try {
      const chosen = multiple ? Array.from(list) : [list[0]];
      for (const f of chosen) {
        const form = new FormData();
        form.append("file", f);
        const res = await upload<StoredFile>("/files", form);
        onUploaded(res.id);
      }
    } catch (e) {
      setUploadErr(e instanceof Error ? e.message : "upload failed");
    } finally {
      setUploading(false);
      if (inputRef.current) inputRef.current.value = "";
    }
  }

  return { inputRef, uploading, uploadErr, handleFiles };
}

/** Single-file field: stores the file id (uuid) as a string. */
export function FileInput({
  value,
  onChange,
}: {
  field: Field;
  value: unknown;
  onChange: (v: unknown) => void;
  error?: string;
}) {
  const id = typeof value === "string" && value ? value : null;
  const { inputRef, uploading, uploadErr, handleFiles } = useFileUpload(
    (fileId) => onChange(fileId),
    false,
  );

  return (
    <div className="space-y-2">
      <input
        ref={inputRef}
        type="file"
        className="hidden"
        onChange={(e) => handleFiles(e.target.files)}
      />
      {id ? (
        <FileCard id={id} onRemove={() => onChange(null)} />
      ) : (
        <button type="button" className={uploadBtn} onClick={() => inputRef.current?.click()} disabled={uploading}>
          <Upload size={14} />
          {uploading ? "Uploading…" : "Upload file"}
        </button>
      )}
      {uploadErr && <p className="text-xs text-red-500 dark:text-red-400">{uploadErr}</p>}
    </div>
  );
}

/** Multi-file field: stores a JSON array of file ids as a string. */
export function FilesInput({
  value,
  onChange,
}: {
  field: Field;
  value: unknown;
  onChange: (v: unknown) => void;
  error?: string;
}) {
  const ids: string[] = (() => {
    if (Array.isArray(value)) return value as string[];
    if (typeof value === "string" && value.trim()) {
      try {
        const parsed = JSON.parse(value);
        return Array.isArray(parsed) ? parsed : [];
      } catch {
        return [];
      }
    }
    return [];
  })();

  const commit = (next: string[]) => onChange(next.length ? JSON.stringify(next) : null);

  const { inputRef, uploading, uploadErr, handleFiles } = useFileUpload(
    (fileId) => commit([...ids, fileId]),
    true,
  );

  return (
    <div className="space-y-2">
      <input
        ref={inputRef}
        type="file"
        multiple
        className="hidden"
        onChange={(e) => handleFiles(e.target.files)}
      />
      {ids.length > 0 && (
        <div className="space-y-2">
          {ids.map((fid, i) => (
            <FileCard key={fid + i} id={fid} onRemove={() => commit(ids.filter((_, idx) => idx !== i))} />
          ))}
        </div>
      )}
      <button type="button" className={uploadBtn} onClick={() => inputRef.current?.click()} disabled={uploading}>
        <Upload size={14} />
        {uploading ? "Uploading…" : "Add file(s)"}
      </button>
      {uploadErr && <p className="text-xs text-red-500 dark:text-red-400">{uploadErr}</p>}
    </div>
  );
}

// ── Dynamic field input ───────────────────────────────────────────────────────

export function FieldInput({
  field,
  value,
  onChange,
  error,
}: {
  field: Field;
  value: unknown;
  onChange: (v: unknown) => void;
  error?: string;
}) {
  const base =
    "w-full rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-2 text-sm text-slate-800 dark:text-slate-100 " +
    "placeholder:text-slate-400 focus:border-brand-400 dark:focus:border-brand-500/60 focus:outline-none focus:ring-2 focus:ring-brand-100 dark:focus:ring-brand-500/20" +
    (error ? " border-red-300 focus:border-red-400 focus:ring-red-100" : "");

  const helperBtn =
    "shrink-0 rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-2 text-xs font-medium text-slate-600 dark:text-slate-300 transition hover:bg-slate-50 dark:hover:bg-white/5 hover:text-slate-800 dark:hover:text-slate-100";

  const str = value == null ? "" : String(value);

  switch (field.type) {
    case "boolean":
      return (
        <div className="flex items-center gap-2">
          <Switch.Root
            checked={!!value}
            onCheckedChange={onChange}
            className={`relative h-5 w-9 rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1 ${
              value ? "bg-brand-600" : "bg-slate-200 dark:bg-white/15"
            }`}
          >
            <Switch.Thumb
              className={`block h-3.5 w-3.5 rounded-full bg-white shadow-sm transition-transform ${
                value ? "translate-x-[18px]" : "translate-x-[3px]"
              }`}
            />
          </Switch.Root>
          <span className="text-sm text-slate-600 dark:text-slate-300">{value ? "Yes" : "No"}</span>
        </div>
      );

    case "text":
      return (
        <textarea
          className={base + " min-h-[120px] resize-y"}
          value={str}
          onChange={(e) => onChange(e.target.value)}
          placeholder={`Enter ${field.display_name.toLowerCase()}…`}
        />
      );

    case "rich_text":
      return (
        <RichTextEditor
          value={str}
          onChange={onChange}
          error={!!error}
          placeholder={`Enter ${field.display_name.toLowerCase()}…`}
        />
      );

    case "json":
      return <JsonInput value={value} onChange={onChange} error={error} />;

    case "uuid":
      return (
        <div className="flex items-center gap-2">
          <input
            type="text"
            className={base + " font-mono"}
            value={str}
            onChange={(e) => onChange(e.target.value || null)}
            placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
          />
          <button
            type="button"
            className={helperBtn}
            onClick={() => onChange(crypto.randomUUID())}
          >
            Generate
          </button>
        </div>
      );

    case "date":
      return (
        <div className="flex items-center gap-2">
          <input
            type="date"
            className={base}
            value={str.slice(0, 10)}
            onChange={(e) => onChange(e.target.value || null)}
          />
          <button
            type="button"
            className={helperBtn}
            onClick={() => onChange(new Date().toISOString().slice(0, 10))}
          >
            Today
          </button>
        </div>
      );

    case "datetime":
    case "timestamp": {
      const local = str ? str.slice(0, 16) : "";
      return (
        <div className="flex items-center gap-2">
          <input
            type="datetime-local"
            className={base}
            value={local}
            onChange={(e) =>
              onChange(e.target.value ? e.target.value.slice(0, 16) + ":00Z" : null)
            }
          />
          <button
            type="button"
            className={helperBtn}
            onClick={() => {
              const now = new Date().toISOString().slice(0, 16) + ":00Z";
              onChange(now);
            }}
          >
            <Clock size={11} className="mr-1 inline" />
            Now
          </button>
        </div>
      );
    }

    case "time":
      return (
        <input
          type="time"
          className={base}
          value={str.slice(0, 8)}
          onChange={(e) => onChange(e.target.value || null)}
        />
      );

    case "integer":
      return (
        <input
          type="number"
          step="1"
          className={base}
          value={str}
          onChange={(e) =>
            onChange(e.target.value === "" ? null : parseInt(e.target.value, 10))
          }
          placeholder="0"
        />
      );

    case "float":
    case "decimal":
      return (
        <input
          type="number"
          step="any"
          className={base}
          value={str}
          onChange={(e) =>
            onChange(e.target.value === "" ? null : parseFloat(e.target.value))
          }
          placeholder="0.0"
        />
      );

    case "select": {
      const choices = (field.options?.choices as { value: string; label: string }[]) ?? [];
      return (
        <Select.Root
          value={String(value ?? "")}
          onValueChange={(v) => onChange(v === "__none__" ? null : v || null)}
        >
          <Select.Trigger className="flex w-full items-center justify-between rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-2 text-sm text-slate-800 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-brand-100 dark:focus:ring-brand-500/20 focus:border-brand-400 dark:focus:border-brand-500/60 [&[data-placeholder]]:text-slate-400">
            <Select.Value placeholder="— select —" />
            <Select.Icon>
              <ChevronDown size={14} className="text-slate-400" />
            </Select.Icon>
          </Select.Trigger>
          <Select.Portal>
            <Select.Content className="z-50 overflow-hidden rounded-xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-800 p-1 shadow-lg">
              <Select.Viewport>
                <Select.Item
                  value="__none__"
                  className="cursor-pointer rounded-lg px-3 py-2 text-sm text-slate-400 outline-none hover:bg-slate-50 dark:hover:bg-white/5 data-[highlighted]:bg-brand-50 dark:data-[highlighted]:bg-brand-500/15 data-[highlighted]:text-brand-700 dark:data-[highlighted]:text-brand-300"
                >
                  <Select.ItemText>— None —</Select.ItemText>
                </Select.Item>
                {choices.map((c) => (
                  <Select.Item
                    key={c.value}
                    value={c.value}
                    className="cursor-pointer rounded-lg px-3 py-2 text-sm text-slate-700 dark:text-slate-200 outline-none hover:bg-slate-50 dark:hover:bg-white/5 data-[highlighted]:bg-brand-50 dark:data-[highlighted]:bg-brand-500/15 data-[highlighted]:text-brand-700 dark:data-[highlighted]:text-brand-300 data-[state=checked]:font-medium"
                  >
                    <Select.ItemText>{c.label}</Select.ItemText>
                  </Select.Item>
                ))}
              </Select.Viewport>
            </Select.Content>
          </Select.Portal>
        </Select.Root>
      );
    }

    case "ip":
      return (
        <input
          type="text"
          className={base + " font-mono"}
          value={str}
          onChange={(e) => onChange(e.target.value || null)}
          placeholder="192.168.1.1 or 2001:db8::1"
        />
      );

    case "file":
      return <FileInput field={field} value={value} onChange={onChange} error={error} />;

    case "files":
      return <FilesInput field={field} value={value} onChange={onChange} error={error} />;

    case "relation":
      return (
        <RelationInput field={field} value={value} onChange={onChange} error={error} />
      );

    default:
      return (
        <input
          type="text"
          className={base}
          value={str}
          onChange={(e) => onChange(e.target.value || null)}
          placeholder={`Enter ${field.display_name.toLowerCase()}…`}
        />
      );
  }
}

// ── Field label ───────────────────────────────────────────────────────────────

export function FieldLabel({ field }: { field: Field }) {
  return (
    <label className="mb-1.5 flex items-center gap-1 text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
      {field.display_name}
      {field.required && <span className="text-red-400">*</span>}
    </label>
  );
}

// ── FieldGroup — renders fields with visible/hidden split ─────────────────────

export function FieldGroup({
  fields,
  values,
  fieldErrors,
  onChange,
  gridClass,
}: {
  fields: Field[];
  values: Record<string, unknown>;
  fieldErrors: Record<string, string>;
  onChange: (name: string, v: unknown) => void;
  gridClass?: string;
}) {
  const [showAdvanced, setShowAdvanced] = useState(false);

  const visibleFields = fields.filter((f) => !f.hidden);
  const hiddenFields = fields.filter((f) => f.hidden);

  const wideTypes = new Set(["text", "rich_text", "json"]);

  function renderField(f: Field, wide?: boolean) {
    return (
      <div key={f.id} className={wide ? "col-span-full" : undefined}>
        <FieldLabel field={f} />
        <FieldInput
          field={f}
          value={values[f.name]}
          onChange={(v) => onChange(f.name, v)}
          error={fieldErrors[f.name]}
        />
        {fieldErrors[f.name] && (
          <p className="mt-1 text-xs text-red-500 dark:text-red-400">{fieldErrors[f.name]}</p>
        )}
        {(f.options as { note?: string })?.note && (
          <p className="mt-1 text-xs text-slate-400">
            {(f.options as { note?: string }).note}
          </p>
        )}
      </div>
    );
  }

  return (
    <div className={gridClass ?? "space-y-4"}>
      {visibleFields.map((f) => renderField(f, wideTypes.has(f.type)))}

      {hiddenFields.length > 0 && (
        <div className="col-span-full">
          <button
            type="button"
            onClick={() => setShowAdvanced((prev) => !prev)}
            className="flex items-center gap-1.5 text-xs font-medium text-slate-400 transition hover:text-slate-600"
          >
            <EyeOff size={13} />
            {showAdvanced ? "Hide" : "Show"} advanced fields
            <span className="ml-0.5 rounded-full bg-slate-100 dark:bg-white/5 px-1.5 py-0.5 text-[10px] text-slate-500 dark:text-slate-400">
              {hiddenFields.length}
            </span>
          </button>

          <AnimatePresence>
            {showAdvanced && (
              <motion.div
                initial={{ opacity: 0, height: 0 }}
                animate={{ opacity: 1, height: "auto" }}
                exit={{ opacity: 0, height: 0 }}
                transition={{ duration: 0.18 }}
                className="overflow-hidden"
              >
                <div
                  className={
                    gridClass
                      ? "mt-4 grid grid-cols-1 gap-5 md:grid-cols-2"
                      : "mt-4 space-y-4"
                  }
                >
                  {hiddenFields.map((f) => renderField(f, wideTypes.has(f.type)))}
                </div>
              </motion.div>
            )}
          </AnimatePresence>
        </div>
      )}
    </div>
  );
}
