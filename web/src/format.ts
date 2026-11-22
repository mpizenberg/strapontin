// French display helpers.

const day = new Intl.DateTimeFormat("fr-FR", { day: "numeric", month: "short", timeZone: "UTC" });
const dayYear = new Intl.DateTimeFormat("fr-FR", { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });
const longDay = new Intl.DateTimeFormat("fr-FR", { weekday: "long", day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });
const monthYear = new Intl.DateTimeFormat("fr-FR", { month: "long", year: "numeric", timeZone: "UTC" });

const parse = (iso: string) => new Date(iso.slice(0, 10) + "T00:00:00Z");

/** Today as YYYY-MM-DD in local time. */
export function today(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

export const formatDay = (iso: string) => dayYear.format(parse(iso));
export const formatLongDay = (iso: string) => capitalize(longDay.format(parse(iso)));
export const formatMonth = (iso: string) => capitalize(monthYear.format(parse(iso)));
export const year = (iso?: string) => iso?.slice(0, 4) ?? "";

export function capitalize(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** "2 → 12 déc. 2026", "jusqu'au 29 nov.", or "" when no date is known. */
export function formatRange(starts?: string, ends?: string): string {
  if (starts && ends && starts !== ends) {
    const sameYear = starts.slice(0, 4) === ends.slice(0, 4);
    return `${sameYear ? day.format(parse(starts)) : formatDay(starts)} → ${formatDay(ends)}`;
  }
  const one = ends ?? starts;
  return one ? formatDay(one) : "";
}

/** Whole days from today to iso; negative when past. */
export function daysUntil(iso: string): number {
  return Math.round((parse(iso).getTime() - parse(today()).getTime()) / 86_400_000);
}

export function plural(n: number, one: string, many: string) {
  return `${n} ${n > 1 ? many : one}`;
}

/** Placeholder background for a production without a poster. */
export const posterClass = (id: number) => ["", "p2", "p3", "p4", "p5"][id % 5];

/** ARIA state attributes take the strings "true" and "false". */
export const aria = (b: boolean) => (b ? "true" : "false");

export function formatBytes(n: number) {
  if (n < 1024) return `${n} o`;
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(0)} Ko`;
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1).replace(".", ",")} Mo`;
  return `${(n / 1024 ** 3).toFixed(2).replace(".", ",")} Go`;
}
