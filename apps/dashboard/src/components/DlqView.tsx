export type DlqEntry = {
    dlq_id: string
    event_id: string
    final_error: string
    moved_at: string
  }
  
  const sampleDlq: DlqEntry[] = [
    { dlq_id: 'dlq_sample_001', event_id: 'evt_sample_004', final_error: 'Webhook signature verification failed after 5 attempts', moved_at: '2026-08-19T13:55:00Z' },
    { dlq_id: 'dlq_sample_002', event_id: 'evt_sample_009', final_error: 'Downstream order service returned HTTP 503: service unavailable', moved_at: '2026-08-19T12:42:00Z' },
    { dlq_id: 'dlq_sample_003', event_id: 'evt_sample_012', final_error: 'Payload validation failed: customer.email is required', moved_at: '2026-08-19T11:18:00Z' },
  ]
  
  export function DlqView({ entries = sampleDlq, onReplay }: { entries?: DlqEntry[]; onReplay: (dlqId: string) => void }) {
    return (
      <section className="rounded-xl border border-red-900/50 bg-zinc-900 shadow-sm">
        <div className="flex items-start justify-between gap-4 border-b border-red-900/40 px-5 py-4">
          <div>
            <p className="font-mono text-[10px] uppercase tracking-[0.2em] text-red-400">Attention / 03</p>
            <h2 className="mt-1 text-base font-semibold text-zinc-100">Dead-letter queue</h2>
          </div>
          <span className="rounded-full bg-red-950/40 px-2 py-1 font-mono text-xs text-red-400">{entries.length} blocked</span>
        </div>
        <div className="divide-y divide-zinc-800">
          {entries.map((entry) => (
            <div key={entry.dlq_id} className="flex flex-col gap-4 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
              <div className="min-w-0">
                <p className="font-mono text-xs text-zinc-500">
                  {entry.dlq_id} <span className="text-zinc-700">/</span> {entry.event_id}
                </p>
                <p className="mt-2 break-words text-sm leading-6 text-zinc-100">{entry.final_error}</p>
                <p className="mt-2 font-mono text-[11px] text-zinc-500">Moved {new Date(entry.moved_at).toLocaleString()}</p>
              </div>
              <button
                onClick={() => onReplay(entry.dlq_id)}
                className="shrink-0 rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-1.5 text-sm font-medium text-zinc-100 hover:bg-zinc-700"
              >
                Replay
              </button>
            </div>
          ))}
        </div>
      </section>
    )
  }