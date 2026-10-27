/** Basic schedule UI ↔ 5-field cron (min hour dom month dow). */
import type { TFunction } from "i18next";

export type ScheduleFrequency = "hourly" | "daily" | "weekly" | "monthly";

export type BasicSchedule = {
  frequency: ScheduleFrequency;
  minute: number; // 0–59
  hour: number; // 0–23 (daily/weekly/monthly)
  weekdays: number[]; // 0=Sun … 6=Sat
  monthDay: number; // 1–31
};

/** `label` / `short` are i18n keys — resolve with t() at the render site. */
export const WEEKDAYS: { value: number; label: string; short: string }[] = [
  { value: 1, label: "workflows.weekday.mon", short: "workflows.weekdayShort.mon" },
  { value: 2, label: "workflows.weekday.tue", short: "workflows.weekdayShort.tue" },
  { value: 3, label: "workflows.weekday.wed", short: "workflows.weekdayShort.wed" },
  { value: 4, label: "workflows.weekday.thu", short: "workflows.weekdayShort.thu" },
  { value: 5, label: "workflows.weekday.fri", short: "workflows.weekdayShort.fri" },
  { value: 6, label: "workflows.weekday.sat", short: "workflows.weekdayShort.sat" },
  { value: 0, label: "workflows.weekday.sun", short: "workflows.weekdayShort.sun" },
];

export const DEFAULT_BASIC: BasicSchedule = {
  frequency: "daily",
  minute: 0,
  hour: 9,
  weekdays: [1, 2, 3, 4, 5],
  monthDay: 1,
};

const FALLBACK_TIMEZONES = [
  "UTC",
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "America/Bogota",
  "America/Mexico_City",
  "America/Sao_Paulo",
  "America/Argentina/Buenos_Aires",
  "Europe/London",
  "Europe/Paris",
  "Europe/Berlin",
  "Europe/Madrid",
  "Africa/Cairo",
  "Asia/Dubai",
  "Asia/Kolkata",
  "Asia/Singapore",
  "Asia/Tokyo",
  "Australia/Sydney",
  "Pacific/Auckland",
];

/** IANA timezones for the schedule picker (browser list when available). */
export function listTimezones(): string[] {
  try {
    const intl = Intl as typeof Intl & {
      supportedValuesOf?: (key: string) => string[];
    };
    if (typeof intl.supportedValuesOf === "function") {
      return intl.supportedValuesOf("timeZone");
    }
  } catch {
    // ignore
  }
  return FALLBACK_TIMEZONES;
}

function clamp(n: number, min: number, max: number) {
  if (!Number.isFinite(n)) return min;
  return Math.min(max, Math.max(min, Math.trunc(n)));
}

export function basicToCron(b: BasicSchedule): string {
  const m = clamp(b.minute, 0, 59);
  const h = clamp(b.hour, 0, 23);
  switch (b.frequency) {
    case "hourly":
      return `${m} * * * *`;
    case "daily":
      return `${m} ${h} * * *`;
    case "weekly": {
      const days = [...new Set(b.weekdays.map((d) => clamp(d, 0, 6)))].sort(
        (a, c) => a - c,
      );
      const dow = days.length ? days.join(",") : "*";
      return `${m} ${h} * * ${dow}`;
    }
    case "monthly":
      return `${m} ${h} ${clamp(b.monthDay, 1, 31)} * *`;
    default:
      return `${m} ${h} * * *`;
  }
}

/** Returns basic schedule if cron matches a pattern we generate; else null. */
export function cronToBasic(cron: string): BasicSchedule | null {
  const parts = cron.trim().split(/\s+/);
  if (parts.length !== 5) return null;
  const [minS, hourS, domS, monthS, dowS] = parts;
  if (monthS !== "*") return null;

  const minute = Number(minS);
  if (!Number.isInteger(minute) || minute < 0 || minute > 59) return null;

  // hourly: M * * * *
  if (hourS === "*" && domS === "*" && dowS === "*") {
    return { ...DEFAULT_BASIC, frequency: "hourly", minute };
  }

  const hour = Number(hourS);
  if (!Number.isInteger(hour) || hour < 0 || hour > 23) return null;

  // daily: M H * * *
  if (domS === "*" && dowS === "*") {
    return { ...DEFAULT_BASIC, frequency: "daily", minute, hour };
  }

  // monthly: M H D * *
  if (dowS === "*" && /^\d+$/.test(domS)) {
    const monthDay = Number(domS);
    if (monthDay < 1 || monthDay > 31) return null;
    return { ...DEFAULT_BASIC, frequency: "monthly", minute, hour, monthDay };
  }

  // weekly: M H * * DOW
  if (domS === "*" && dowS !== "*") {
    const weekdays: number[] = [];
    for (const chunk of dowS.split(",")) {
      if (chunk.includes("-")) {
        const [a, b] = chunk.split("-").map(Number);
        if (!Number.isInteger(a) || !Number.isInteger(b)) return null;
        for (let d = a; d <= b; d++) weekdays.push(d === 7 ? 0 : d);
      } else {
        const d = Number(chunk);
        if (!Number.isInteger(d)) return null;
        weekdays.push(d === 7 ? 0 : d);
      }
    }
    if (weekdays.some((d) => d < 0 || d > 6)) return null;
    return {
      ...DEFAULT_BASIC,
      frequency: "weekly",
      minute,
      hour,
      weekdays: [...new Set(weekdays)].sort((a, b) => a - b),
    };
  }

  return null;
}

export function describeBasic(b: BasicSchedule, t: TFunction): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  const time = `${pad(b.hour)}:${pad(b.minute)}`;
  switch (b.frequency) {
    case "hourly":
      return t("workflows.schedule.describeHourly", { minute: pad(b.minute) });
    case "daily":
      return t("workflows.schedule.describeDaily", { time });
    case "weekly": {
      const labels = WEEKDAYS.filter((d) => b.weekdays.includes(d.value))
        .map((d) => t(d.short))
        .join(", ");
      return t("workflows.schedule.describeWeekly", {
        days: labels || "—",
        time,
      });
    }
    case "monthly":
      return t("workflows.schedule.describeMonthly", {
        day: b.monthDay,
        time,
      });
    default:
      return "";
  }
}

export type FilterRule = { id: string; field: string; value: string };

export function filterToRules(filter: unknown): FilterRule[] {
  if (!filter || typeof filter !== "object" || Array.isArray(filter)) return [];
  return Object.entries(filter as Record<string, unknown>).map(
    ([field, value], i) => ({
      id: `r${i}-${field}`,
      field,
      value: value == null ? "" : String(value),
    }),
  );
}

export function rulesToFilter(
  rules: FilterRule[],
): Record<string, string> | undefined {
  const out: Record<string, string> = {};
  for (const r of rules) {
    const field = r.field.trim();
    if (!field) continue;
    out[field] = r.value;
  }
  return Object.keys(out).length ? out : undefined;
}
