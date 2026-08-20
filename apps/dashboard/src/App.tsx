import { EventsTable } from './components/EventsTable'
import { OrdersTable } from './components/OrdersTable'
import { DlqView } from './components/DlqView'

function App() {
  return (
    <div className="min-h-screen bg-zinc-950 p-8">
      <div className="mx-auto max-w-5xl space-y-6">
        <h1 className="text-xl font-semibold text-zinc-100">Relay / Operations</h1>
        <EventsTable />
        <OrdersTable />
        <DlqView onReplay={(dlqId) => console.log('replay clicked:', dlqId)} />
      </div>
    </div>
  )
}

export default App