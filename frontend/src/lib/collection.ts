/**
 * Helpers for the collection → sections → fields schema shape returned by
 * GET /collections/{name}. Fields are never top-level on the collection JSON.
 */
import type { Collection, Condition, Field, Section } from "@/types";

/** Flatten every field across all sections (section then field sort order). */
export function allFields(collection: Collection | undefined | null): Field[] {
  if (!collection?.sections?.length) return [];
  const sections = [...collection.sections].sort((a, b) => a.sort - b.sort);
  const out: Field[] = [];
  for (const s of sections) {
    const fields = [...(s.fields ?? [])].sort((a, b) => a.sort - b.sort);
    out.push(...fields);
  }
  return out;
}

/** Sections sorted for form layout. */
export function sortedSections(collection: Collection | undefined | null): Section[] {
  if (!collection?.sections?.length) return [];
  return [...collection.sections].sort((a, b) => a.sort - b.sort);
}

function isEmpty(v: unknown): boolean {
  if (v == null) return true;
  if (typeof v === "string") return v === "";
  if (Array.isArray(v)) return v.length === 0;
  if (typeof v === "object") return Object.keys(v as object).length === 0;
  return false;
}

/** Evaluate a section visibility condition. null / omitted = always visible. */
export function evalCond(
  c: Condition | null | undefined,
  values: Record<string, unknown>,
): boolean {
  if (c == null) return true;
  if ("and" in c) return c.and.every((x) => evalCond(x, values));
  if ("or" in c) return c.or.some((x) => evalCond(x, values));
  const actual = values[c.field];
  switch (c.op) {
    case "empty":
      return isEmpty(actual);
    case "nempty":
      return !isEmpty(actual);
    case "eq":
      return String(actual) === String(c.value);
    case "neq":
      return String(actual) !== String(c.value);
    case "in":
      return Array.isArray(c.value) && c.value.some((x) => String(x) === String(actual));
    case "nin":
      return Array.isArray(c.value) && !c.value.some((x) => String(x) === String(actual));
    case "contains":
      return String(actual ?? "").includes(String(c.value ?? ""));
    case "gt":
      return Number(actual) > Number(c.value);
    case "gte":
      return Number(actual) >= Number(c.value);
    case "lt":
      return Number(actual) < Number(c.value);
    case "lte":
      return Number(actual) <= Number(c.value);
    default:
      return false;
  }
}

/** Evaluate a section visibility condition against current form values. */
export function isSectionVisible(
  visibility: Condition | null | undefined,
  values: Record<string, unknown>,
): boolean {
  return evalCond(visibility, values);
}

/** Non-system fields from sections that currently pass visibility. */
export function visibleSectionFields(
  sections: Section[],
  values: Record<string, unknown>,
): Field[] {
  const out: Field[] = [];
  for (const s of sections) {
    if (!isSectionVisible(s.visibility, values)) continue;
    for (const f of [...(s.fields ?? [])].sort((a, b) => a.sort - b.sort)) {
      if (!f.is_system) out.push(f);
    }
  }
  return out;
}

/**
 * After a flat field-list response (e.g. reorder), merge fields back into the
 * collection's sections by matching `field.section`.
 */
export function mergeFieldsIntoSections(
  collection: Collection,
  fields: Field[],
): Collection {
  const bySection = new Map<string, Field[]>();
  for (const f of fields) {
    const key = f.section || "general";
    const list = bySection.get(key) ?? [];
    list.push(f);
    bySection.set(key, list);
  }
  return {
    ...collection,
    sections: (collection.sections ?? []).map((s) => ({
      ...s,
      fields: bySection.get(s.name) ?? [],
    })),
  };
}
