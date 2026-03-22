import { useEffect, useRef, useCallback } from 'react'
import { api } from '../api/client'
import type { WSChatMessage, WSTyping, WSPresence } from '../types'

type WSHandlers = {
  onMessage:  (msg: WSChatMessage) => void
  onTyping:   (msg: WSTyping)      => void
  onPresence: (msg: WSPresence)    => void
}

async function fetchWSTicket(): Promise<string> {
  const data = await api.post<{ ticket: string }>('/api/v1/ws/ticket', {})
  return data.ticket
}

export function useWebSocket(handlers: WSHandlers) {
  const ws           = useRef<WebSocket | null>(null)
  const reconnTimer  = useRef<ReturnType<typeof setTimeout> | null>(null)
  const handlersRef  = useRef(handlers)

  useEffect(() => { handlersRef.current = handlers }, [handlers])

  const connect = useCallback(async () => {
    if (ws.current)      ws.current.close()
    if (reconnTimer.current) clearTimeout(reconnTimer.current)

    let ticket: string
    try {
      ticket = await fetchWSTicket()
    } catch {
      // Not authenticated yet — retry after delay
      reconnTimer.current = setTimeout(connect, 3000)
      return
    }

    const base   = (import.meta.env.VITE_API_URL ?? window.location.origin)
      .replace(/^https/, 'wss')
      .replace(/^http/,  'ws')
    const socket = new WebSocket(`${base}/api/v1/ws?ticket=${encodeURIComponent(ticket)}`)
    ws.current   = socket

    socket.addEventListener('message', (e) => {
      try {
        const msg = JSON.parse(e.data)
        switch (msg.type) {
          case 'message':  handlersRef.current.onMessage(msg);  break
          case 'typing':   handlersRef.current.onTyping(msg);   break
          case 'presence': handlersRef.current.onPresence(msg); break
        }
      } catch { /* ignore malformed */ }
    })

    socket.addEventListener('close', () => {
      reconnTimer.current = setTimeout(connect, 3000)
    })

    socket.addEventListener('error', () => {
      socket.close()
    })
  }, [])

  useEffect(() => {
    connect()
    return () => {
      if (reconnTimer.current) clearTimeout(reconnTimer.current)
      ws.current?.close()
    }
  }, [connect])

  const send = useCallback((data: unknown) => {
    if (ws.current?.readyState === WebSocket.OPEN) {
      ws.current.send(JSON.stringify(data))
    }
  }, [])

  return { send }
}