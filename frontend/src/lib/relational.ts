/**
 * relational.ts — client helpers for the backend's generic relational item I/O.
 *
 * Mirrors the API contract in the backend docs (relational-items.md): discover a
 * collection's child (one-to-many) relations from metadata, read an item with its
 * children expanded (`?deep=`), and let callers send nested child arrays on save
 * (the backend reconciles insert/update/delete in one transaction).
 */
import { get } from "@/lib/api";
import type { Field, Relation } from "@/types";

/** A one-to-many relation where a given collection is the parent (one) side. */
export interface ChildRelation {
  relation: Relation;
  /** Child collection that holds the FK (relation.collection_many). */
  childCollection: string;
  /** FK column on the child pointing back at the parent (relation.field_many). */
  fkColumn: string;
  /** True when the child is owned (on_delete=cascade): omitted rows are deleted. */
  owned: boolean;
}

/** GET the relations declared for a collection. */
export const listRelations = (collection: string) =>
  get<Relation[]>(`/collections/${collection}/relations`);

/**
 * childRelationsOf returns the relations for which `collection` is the parent
 * (one) side and which are expandable/writable as nested arrays — i.e. plain
 * one-to-many, excluding pure many-to-many (junction) relations the nested-write
 * engine does not yet handle.
 */
export function childRelationsOf(relations: Relation[], collection: string): ChildRelation[] {
  return relations
    .filter((r) => r.collection_one === collection && !r.junction_table)
    .map((r) => ({
      relation: r,
      childCollection: r.collection_many,
      fkColumn: r.field_many,
      owned: (r.on_delete ?? "").toLowerCase() === "cascade",
    }));
}

/**
 * withRelationOptions returns `fields` where each UUID field that is a foreign
 * key of a relation on `collection` (i.e. a `collection_many === collection`
 * relation's `field_many`) carries `options.related_collection`, so the field
 * renders as a relation picker instead of a raw UUID text box. Existing options
 * (including a configured `display_field`) are preserved; a field that already
 * has `related_collection` is left untouched. This is what turns a blueprint FK
 * column like `website_pages_blocks.block_id` into a Blocks picker.
 */
export function withRelationOptions(
  fields: Field[],
  relations: Relation[],
  collection: string,
): Field[] {
  const fkToOne = new Map<string, string>();
  for (const r of relations) {
    if (r.collection_many === collection && !r.junction_table) {
      fkToOne.set(r.field_many, r.collection_one);
    }
  }
  if (fkToOne.size === 0) return fields;
  return fields.map((f) => {
    const relatedCollection = fkToOne.get(f.name);
    if (!relatedCollection) return f;
    const opts = (f.options as Record<string, unknown> | undefined) ?? {};
    if (opts.related_collection) return f;
    return { ...f, options: { ...opts, related_collection: relatedCollection } };
  });
}

/** GET one item with the named child relations expanded inline. */
export const getItemDeep = (
  collection: string,
  id: string,
  deep: string[],
  lang?: string,
) =>
  get<Record<string, unknown>>(`/items/${collection}/${id}`, {
    ...(deep.length ? { deep: deep.join(",") } : {}),
    ...(lang ? { lang } : {}),
  });
