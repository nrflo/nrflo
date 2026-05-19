import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Table, TableHeader, TableBody, TableHead, TableRow } from '@/components/ui/Table'
import { useConnectionsStore } from '@/stores/connectionsStore'
import { ConnectionRow } from './ConnectionRow'
import { ConnectionAddDialog } from './ConnectionAddDialog'

export function ConnectionsPage() {
  const { list, activeId } = useConnectionsStore()
  const [addOpen, setAddOpen] = useState(false)

  return (
    <div className="p-6 max-w-4xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Connections</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Manage connections to nrflo server instances. Remote servers must allow CORS from this origin.
          </p>
        </div>
        <Button size="sm" onClick={() => setAddOpen(true)}>Add connection</Button>
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Base URL</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Test</TableHead>
            <TableHead />
          </TableRow>
        </TableHeader>
        <TableBody>
          {list.map((conn) => (
            <ConnectionRow key={conn.id} conn={conn} isActive={conn.id === activeId} />
          ))}
        </TableBody>
      </Table>

      <ConnectionAddDialog open={addOpen} onClose={() => setAddOpen(false)} />
    </div>
  )
}
