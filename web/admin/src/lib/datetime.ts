// Scheduling is entered and shown in Japan time. Japan has no daylight saving time,
// so a fixed +09:00 offset is exact.
const JST_OFFSET_MS = 9 * 60 * 60 * 1000;

const longFormat = new Intl.DateTimeFormat("ja-JP", {
  year: "numeric",
  month: "long",
  day: "numeric",
  hour: "numeric",
  minute: "2-digit",
  hourCycle: "h23",
  timeZone: "Asia/Tokyo",
});

/** "2026年12月1日 9:00" in Japan time. */
export const formatJapanDateTime = (iso: string) => longFormat.format(new Date(iso));

const shortFormat = new Intl.DateTimeFormat("ja-JP", {
  dateStyle: "medium",
  timeStyle: "short",
  hourCycle: "h23",
  timeZone: "Asia/Tokyo",
});

/** "2026/11/20 10:00" in Japan time, or "" for null. For lists and 「最終更新」. */
export const formatDateTime = (iso: string | null) =>
  iso ? shortFormat.format(new Date(iso)) : "";

/** Splits an instant into Japan-time values for <input type="date"> and <input type="time">. */
export function toJapanInputs(iso: string): { date: string; time: string } {
  const local = new Date(new Date(iso).getTime() + JST_OFFSET_MS).toISOString();
  return { date: local.slice(0, 10), time: local.slice(11, 16) };
}

/** Japan-time date ("2026-12-01") and time ("09:00") as RFC 3339 in UTC, or null if invalid. */
export function fromJapanInputs(date: string, time: string): string | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || !/^\d{2}:\d{2}$/.test(time)) return null;
  const d = new Date(`${date}T${time}:00+09:00`);
  if (Number.isNaN(d.getTime())) return null;
  return d.toISOString().replace(".000Z", "Z");
}

/** Default for a new schedule: tomorrow at 9:00 Japan time. */
export function defaultScheduleInputs(now = new Date()): { date: string; time: string } {
  const tomorrow = toJapanInputs(new Date(now.getTime() + 24 * 60 * 60 * 1000).toISOString());
  return { date: tomorrow.date, time: "09:00" };
}
