export type WebhookEvent = {
    event_id: string
    idempotency_key: string
    source: string
    status: string
    received_at: string
  }
  
  const sampleEvents: WebhookEvent[] = [
    { event_id: 'evt_sample_001', idempotency_key: 'idem_checkout_7f2a', source: 'stripe', status: 'completed', received_at: '2026-08-19T14:32:00Z' },
    { event_id: 'evt_sample_002', idempotency_key: 'idem_payment_91bd', source: 'stripe', status: 'processing', received_at: '2026-08-19T14:28:00Z' },
    { event_id: 'evt_sample_003', idempotency_key: 'idem_order_44c1', source: 'shopify', status: 'pending', received_at: '2026-08-19T14:21:00Z' },
    { event_id: 'evt_sample_004', idempotency_key: 'idem_refund_a823', source: 'stripe', status: 'dead_letter', received_at: '2026-08-19T13:54:00Z' },
  ]
  
  const statusStyles: Record<string, string> = {
    completed: 'bg-emerald-600/20 text-emerald-400',
    processing: 'bg-blue-600/20 text-blue-400',
    pending: 'bg-zinc-700/50 text-zinc-300',
    dead_letter: 'bg-red-600/20 text-red-400',
  }
  
  export function EventsTable({ events = sampleEvents }: { events?: WebhookEvent[] }) {
    return (
      <section className="rounded-xl border border-zinc-800 bg-zinc-900 shadow-sm">
        <div className="flex items-start justify-between gap-4 border-b border-zinc-800 px-5 py-4">
          <div>
            <p className="font-mono text-[10px] uppercase tracking-[0.2em] text-zinc-500">Stream / 01</p>
            <h2 className="mt-1 text-base font-semibold text-zinc-100">Webhook events</h2>
          </div>
          <span className="font-mono text-xs text-zinc-500">{events.length} total</span>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[680px] text-left text-sm">
            <thead className="bg-zinc-800/40 font-mono text-[10px] uppercase tracking-wider text-zinc-500">
              <tr>
                <th className="px-5 py-3 font-medium">Event / key</th>
                <th className="px-5 py-3 font-medium">Source</th>
                <th className="px-5 py-3 font-medium">Status</th>
                <th className="px-5 py-3 font-medium">Received</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800">
              {events.map((event) => (
                <tr key={event.event_id} className="hover:bg-zinc-800/30">
                  <td className="px-5 py-3">
                    <div className="font-mono text-xs text-zinc-100">{event.event_id}</div>
                    <div className="mt-1 font-mono text-[11px] text-zinc-500">{event.idempotency_key}</div>
                  </td>
                  <td className="px-5 py-3 text-zinc-400">{event.source}</td>
                  <td className="px-5 py-3">
                    <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${statusStyles[event.status] ?? 'bg-zinc-700/50 text-zinc-300'}`}>
                      {event.status}
                    </span>
                  </td>
                  <td className="px-5 py-3 font-mono text-xs text-zinc-500">{new Date(event.received_at).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    )
  }