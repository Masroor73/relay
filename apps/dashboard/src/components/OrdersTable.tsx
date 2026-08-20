export type ProcessedOrder = {
    order_id: string
    stripe_event_id: string
    amount_cents: number
    status: string
    created_at: string
  }
  
  const sampleOrders: ProcessedOrder[] = [
    { order_id: 'ord_sample_001', stripe_event_id: 'evt_stripe_8c42', amount_cents: 12900, status: 'completed', created_at: '2026-08-19T14:31:00Z' },
    { order_id: 'ord_sample_002', stripe_event_id: 'evt_stripe_1d09', amount_cents: 4900, status: 'completed', created_at: '2026-08-19T14:10:00Z' },
    { order_id: 'ord_sample_003', stripe_event_id: 'evt_stripe_77ab', amount_cents: 28750, status: 'processing', created_at: '2026-08-19T13:48:00Z' },
  ]
  
  const statusStyles: Record<string, string> = {
    completed: 'bg-emerald-600/20 text-emerald-400',
    processing: 'bg-blue-600/20 text-blue-400',
  }
  
  export function OrdersTable({ orders = sampleOrders }: { orders?: ProcessedOrder[] }) {
    return (
      <section className="rounded-xl border border-zinc-800 bg-zinc-900 shadow-sm">
        <div className="flex items-start justify-between gap-4 border-b border-zinc-800 px-5 py-4">
          <div>
            <p className="font-mono text-[10px] uppercase tracking-[0.2em] text-zinc-500">Ledger / 02</p>
            <h2 className="mt-1 text-base font-semibold text-zinc-100">Processed orders</h2>
          </div>
          <span className="font-mono text-xs text-zinc-500">{orders.length} total</span>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[620px] text-left text-sm">
            <thead className="bg-zinc-800/40 font-mono text-[10px] uppercase tracking-wider text-zinc-500">
              <tr>
                <th className="px-5 py-3 font-medium">Order</th>
                <th className="px-5 py-3 font-medium">Amount</th>
                <th className="px-5 py-3 font-medium">Status</th>
                <th className="px-5 py-3 font-medium">Created</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800">
              {orders.map((order) => (
                <tr key={order.order_id} className="hover:bg-zinc-800/30">
                  <td className="px-5 py-3">
                    <div className="font-mono text-xs text-zinc-100">{order.order_id}</div>
                    <div className="mt-1 font-mono text-[11px] text-zinc-500">{order.stripe_event_id}</div>
                  </td>
                  <td className="px-5 py-3 font-mono text-sm text-zinc-100">
                    {new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(order.amount_cents / 100)}
                  </td>
                  <td className="px-5 py-3">
                    <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${statusStyles[order.status] ?? 'bg-zinc-700/50 text-zinc-300'}`}>
                      {order.status}
                    </span>
                  </td>
                  <td className="px-5 py-3 font-mono text-xs text-zinc-500">{new Date(order.created_at).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    )
  }