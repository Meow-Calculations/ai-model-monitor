export const THEME_MODES = ['system', 'time', 'light', 'dark']
export const THEME_MODE_KEY = 'ai-model-monitor.theme.mode'
export const THEME_TIME_RANGE_KEY = 'ai-model-monitor.theme.timeRange'

export const DEFAULT_TIME_RANGE = {
  darkStart: '18:00',
  darkEnd: '07:00',
}

export function getStoredThemeMode() {
  const mode = localStorage.getItem(THEME_MODE_KEY)
  return THEME_MODES.includes(mode) ? mode : 'system'
}

export function setStoredThemeMode(mode) {
  if (THEME_MODES.includes(mode)) {
    localStorage.setItem(THEME_MODE_KEY, mode)
  }
}

export function getStoredTimeRange() {
  try {
    const parsed = JSON.parse(localStorage.getItem(THEME_TIME_RANGE_KEY) || '')
    if (isTimeValue(parsed?.darkStart) && isTimeValue(parsed?.darkEnd)) {
      return parsed
    }
  } catch {
    // Use default range.
  }
  return DEFAULT_TIME_RANGE
}

export function setStoredTimeRange(range) {
  localStorage.setItem(THEME_TIME_RANGE_KEY, JSON.stringify({
    darkStart: isTimeValue(range.darkStart) ? range.darkStart : DEFAULT_TIME_RANGE.darkStart,
    darkEnd: isTimeValue(range.darkEnd) ? range.darkEnd : DEFAULT_TIME_RANGE.darkEnd,
  }))
}

export function resolveTheme(mode, timeRange, now = new Date()) {
  if (mode === 'light' || mode === 'dark') return mode
  if (mode === 'time') return isInDarkRange(now, timeRange) ? 'dark' : 'light'
  return getSystemTheme()
}

export function getSystemTheme() {
  if (window.matchMedia?.('(prefers-color-scheme: dark)').matches) return 'dark'
  return 'light'
}

export function isInDarkRange(now, range = DEFAULT_TIME_RANGE) {
  const start = minutesFromTime(range.darkStart || DEFAULT_TIME_RANGE.darkStart)
  const end = minutesFromTime(range.darkEnd || DEFAULT_TIME_RANGE.darkEnd)
  const current = now.getHours() * 60 + now.getMinutes()

  if (start === end) return true
  if (start < end) return current >= start && current < end
  return current >= start || current < end
}

function minutesFromTime(value) {
  if (!isTimeValue(value)) return 0
  const [hours, minutes] = value.split(':').map(Number)
  return hours * 60 + minutes
}

function isTimeValue(value) {
  return typeof value === 'string' && /^([01]\d|2[0-3]):[0-5]\d$/.test(value)
}
