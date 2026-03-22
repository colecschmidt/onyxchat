import { useAuth } from './context/AuthContext'
import { AuthScreen } from './components/AuthScreen'
import { Sidebar } from './components/Sidebar'
import { ChatPanel } from './components/ChatPanel'

export default function App() {
  const { isAuthenticated } = useAuth()

  if (!isAuthenticated) return <AuthScreen />

  return (
    <div style={{ display: 'flex', height: '100vh', width: '100%', overflow: 'hidden' }}>
      <Sidebar />
      <ChatPanel />
    </div>
  )
}