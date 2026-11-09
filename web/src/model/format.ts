// Display formatting shared by every screen.
import { formatDashboardDateTime, type DashboardLanguage } from '@appthrust/platform-dashboard-i18n'

/** `sha256:0123456789abcdef…` → `sha256:0123456789ab`; the full value stays in `title`. */
export function shortDigest(digest: string): string {
  const at = digest.indexOf(':')
  return at === -1 ? digest.slice(0, 12) : `${digest.slice(0, at + 1)}${digest.slice(at + 1, at + 13)}`
}

export function formatTime(language: DashboardLanguage, iso: string): string {
  return formatDashboardDateTime(language, iso, { dateStyle: 'medium', timeStyle: 'medium' })
}

/** "3 minutes ago" in the display language, rounded to the largest whole unit. */
export function formatAge(locale: string, iso: string, now = Date.now()): string {
  const seconds = Math.round((Date.parse(iso) - now) / 1000)
  const format = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
  const abs = Math.abs(seconds)
  if (abs < 60) return format.format(seconds, 'second')
  if (abs < 3600) return format.format(Math.round(seconds / 60), 'minute')
  if (abs < 86_400) return format.format(Math.round(seconds / 3600), 'hour')
  return format.format(Math.round(seconds / 86_400), 'day')
}

export function formatDuration(locale: string, ms: number): string {
  if (ms < 1000) return `${new Intl.NumberFormat(locale).format(Math.round(ms))} ms`
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(ms / 1000)} s`
}
