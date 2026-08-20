import type { WebhookEvent } from '../components/EventsTable'
import type { ProcessedOrder } from '../components/OrdersTable'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080'
const STORAGE_KEY = 'relay_dashboard_password'

export class ApiAuthError extends Error {}

// sessionStorage, not localStorage — cleared when the tab closes, which
// is the right lifetime for a shared credential on an internal tool,
// rather than persisting indefinitely on the machine.
export function setStoredPassword(password: string) {
  sessionStorage.setItem(STORAGE_KEY, password)
}

export function clearStoredPassword() {
  sessionStorage.removeItem(STORAGE_KEY)
}

function getStoredPassword(): string | null {
  return sessionStorage.getItem(STORAGE_KEY)
}

async function apiFetch<T>(path: string): Promise<T> {
  const password = getStoredPassword()
  if (!password) throw new ApiAuthError('No password set')

  // Basic Auth requires a username; our middleware (ENG-35) only checks
  // the password half, so the username value itself is arbitrary.
  const response = await fetch(`${API_BASE_URL}${path}`, {
    headers: { Authorization: 'Basic ' + btoa(`dashboard:${password}`) },
  })

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