import * as Sentry from '@sentry/react'

// Sentry is disabled unless VITE_SENTRY_DSN is set, so the app runs fine
// without it configured (e.g. local dev).
export function initSentry() {
  const dsn = import.meta.env.VITE_SENTRY_DSN
  if (!dsn) return

  Sentry.init({
    dsn,
    environment: import.meta.env.MODE,
  })
}

export { Sentry }
