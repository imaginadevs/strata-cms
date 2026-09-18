import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

/** Merge Tailwind classes safely (handles conflicts). */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/** Format a UTC ISO string to locale date+time. */
export function formatDate(iso: string, locale = "en-US"): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(iso));
}

/**
 * Normalise an item id from the API. pgx sometimes surfaces UUIDs as a 16-byte
 * array which JSON-encodes to numbers; String(id) then becomes "57,67,…".
 */
export function itemId(value: unknown): string {
  if (value == null) return "";
  if (typeof value === "string") return value;
  if (
    Array.isArray(value) &&
    value.length === 16 &&
    value.every((n) => typeof n === "number")
  ) {
    const hex = (value as number[])
      .map((b) => (b & 0xff).toString(16).padStart(2, "0"))
      .join("");
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
  }
  return String(value);
}

/** Truncate a string to n characters with ellipsis. */
export function truncate(str: string, n: number): string {
  return str.length > n ? str.slice(0, n - 1) + "…" : str;
}

/** Convert snake_case to Title Case. */
export function toTitle(str: string): string {
  return str
    .split("_")
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}

/** Generate initials from a full name. */
export function initials(name: string): string {
  return name
    .split(" ")
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase() ?? "")
    .join("");
}

/** Deep-clone a plain object (no functions/symbols). */
export function deepClone<T>(obj: T): T {
  return JSON.parse(JSON.stringify(obj));
}
