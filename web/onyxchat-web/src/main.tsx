import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { AuthProvider } from './context/AuthContext.tsx'
import { ChatProvider } from './context/ChatContext.tsx'
import { initSentry, Sentry } from './lib/sentry.ts'

initSentry()

// Service worker registration lives here rather than in index.html so it can be
// gated on the build mode. In dev the worker's caches shadow Vite's module
// graph, which shows up as edits silently not appearing in the browser — so dev
// not only skips registration but tears down any worker a previous prod build
// (or an earlier version of this file) left installed on localhost.
if ('serviceWorker' in navigator) {
  if (import.meta.env.PROD) {
    navigator.serviceWorker.register('/sw.js').catch(console.error)
  } else {
    navigator.serviceWorker.getRegistrations()
      .then(regs => Promise.all(regs.map(r => r.unregister())))
      .then(() => caches?.keys().then(keys => Promise.all(keys.map(k => caches.delete(k)))))
      .catch(console.error)
  }
}

const isAdmin = window.location.pathname === '/admin'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Sentry.ErrorBoundary fallback={<p>Something went wrong. Please reload the page.</p>}>
      <AuthProvider>
        {isAdmin ? <App /> : <ChatProvider><App /></ChatProvider>}
      </AuthProvider>
    </Sentry.ErrorBoundary>
  </StrictMode>,
)