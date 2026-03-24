// src/App.tsx
import { useAuth } from './context/AuthContext'
import { AuthScreen } from './components/AuthScreen'
import { Sidebar } from './components/Sidebar'
import { ChatPanel } from './components/ChatPanel'
import { AdminPanel } from './components/AdminPanel'

export default function App() {
  const { isAuthenticated } = useAuth()

  if (window.location.pathname === '/admin') {
    if (!isAuthenticated) return <AuthScreen />
    return <AdminPanel />
  }

  if (!isAuthenticated) return <AuthScreen />

  return (
    <div style={{ display: 'flex', height: '100vh', width: '100%', overflow: 'hidden' }}>
      <Sidebar />
      <ChatPanel />
    </div>
  )
}