import { useState, type FormEvent } from 'react'
import { setStoredPassword } from '../lib/api'

export function PasswordGate({ onSubmit, error }: { onSubmit: () => void; error?: string | null }) {
  const [password, setPassword] = useState('')

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setStoredPassword(password)
    onSubmit()
  }

  return (
    <form onSubmit={handleSubmit} className="flex min-h-screen items-center justify-center bg-zinc-950">
      <div className="w-full max-w-sm space-y-4 rounded-xl border border-zinc-800 bg-zinc-900 p-6">
        <h1 className="text-lg font-semibold text-zinc-100">Relay Dashboard</h1>
        <input
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder="Dashboard password"
          autoFocus
          className="w-full rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100"
        />
        {error && <p className="text-sm text-red-400">{error}</p>}
        <button type="submit" className="w-full rounded-lg bg-zinc-100 px-3 py-2 text-sm font-medium text-zinc-900">
          Unlock
        </button>
      </div>
    </form>
  )
}