import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { AuthProvider } from './context/AuthContext.tsx'
import { ChatProvider } from './context/ChatContext.tsx'
import { initSentry, Sentry } from './lib/sentry.ts'

initSentry()

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