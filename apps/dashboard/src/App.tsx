import { useEffect, useState } from 'react'
import { EventsTable, type WebhookEvent } from './components/EventsTable'
import { OrdersTable, type ProcessedOrder } from './components/OrdersTable'
import { DlqView, type DlqEntry } from './components/DlqView'
import { PasswordGate } from './components/PasswordGate'
import { fetchEvents, fetchOrders, fetchDlq, replayDlqEntry, ApiAuthError } from './lib/api'

function App() {
  const [unlocked, setUnlocked] = useState(false)
  const [authError, setAuthError] = useState<string | null>(null)
  const [events, setEvents] = useState<WebhookEvent[] | null>(null)
  const [orders, setOrders] = useState<ProcessedOrder[] | null>(null)
  const [dlq, setDlq] = useState<DlqEntry[] | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)

  function loadDlq() {
    fetchDlq()
      .then(setDlq)
      .catch((err) => setLoadError(err instanceof Error ? err.message : 'Failed to load DLQ'))
  }

  useEffect(() => {
    if (!unlocked) return

    Promise.all([fetchEvents(), fetchOrders(), fetchDlq()])
      .then(([eventsData, ordersData, dlqData]) => {
        setEvents(eventsData)
        setOrders(ordersData)
        setDlq(dlqData)
      })
      .catch((err) => {
        if (err instanceof ApiAuthError) {
          setUnlocked(false)
          setAuthError('Incorrect password')
        } else {
          setLoadError(err instanceof Error ? err.message : 'Failed to load')
        }
      })
  }, [unlocked])

  async function handleReplay(dlqId: string) {
    if (!window.confirm('Replay this event? It will be moved back into the processing queue.')) {
      return
    }
    try {
      await replayDlqEntry(dlqId)
      loadDlq()
    } catch (err) {
      if (err instanceof ApiAuthError) {
        setUnlocked(false)
        setAuthError('Incorrect password')
      } else {
        setLoadError(err instanceof Error ? err.message : 'Replay failed')
      }
    }
  }

  if (!unlocked) {
    return <PasswordGate onSubmit={() => setUnlocked(true)} error={authError} />
  }

  return (
    <div className="min-h-screen bg-zinc-950 p-8">
      <div className="mx-auto max-w-5xl space-y-6">
        <h1 className="text-xl font-semibold text-zinc-100">Relay / Operations</h1>
        {loadError && <p className="text-red-400">{loadError}</p>}
        {events === null || orders === null || dlq === null ? (
          <p className="text-zinc-500">Loading…</p>
        ) : (
          <>
            <EventsTable events={events} />
            <OrdersTable orders={orders} />
            <DlqView entries={dlq} onReplay={handleReplay} />
          </>
        )}
      </div>
    </div>
  )
}

export default App