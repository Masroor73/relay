import type { WebhookEvent } from '../components/EventsTable'
import type { ProcessedOrder } from '../components/OrdersTable'
import type { DlqEntry } from '../components/DlqView'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080'
const STORAGE_KEY = 'relay_dashboard_password'

export class ApiAuthError extends Error {}

export function setStoredPassword(password: string) {
  sessionStorage.setItem(STORAGE_KEY, password)
}

export function clearStoredPassword() {
  sessionStorage.removeItem(STORAGE_KEY)
}

function getStoredPassword(): string | null {
  return sessionStorage.getItem(STORAGE_KEY)
}

function authHeader(): HeadersInit {
  const password = getStoredPassword()
  if (!password) throw new ApiAuthError('No password set')
  return { Authorization: 'Basic ' + btoa(`dashboard:${password}`) }
}

async function apiFetch<T>(path: string): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, { headers: authHeader() })

  if (response.status === 401) {
    clearStoredPassword()
    throw new ApiAuthError('Invalid password')
  }
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status}`)
  }

  return response.json() as Promise<T>
}

export const fetchEvents = () => apiFetch<WebhookEvent[]>('/api/events')
export const fetchOrders = () => apiFetch<ProcessedOrder[]>('/api/orders')
export const fetchDlq = () => apiFetch<DlqEntry[]>('/api/dlq')

export async function replayDlqEntry(dlqId: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/dlq/${dlqId}/replay`, {
    method: 'POST',
    headers: authHeader(),
  })

  if (response.status === 401) {
    clearStoredPassword()
    throw new ApiAuthError('Invalid password')
  }
  if (!response.ok) {
    throw new Error(`Replay failed: ${response.status}`)
  }
}