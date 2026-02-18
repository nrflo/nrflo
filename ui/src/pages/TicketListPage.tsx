import { useSearchParams, Link } from 'react-router-dom'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { TicketList } from '@/components/tickets/TicketList'
import { useTicketList, useTicketSearch } from '@/hooks/useTickets'

export function TicketListPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const statusFilter = searchParams.get('status') || ''
  const typeFilter = searchParams.get('type') || ''
  const searchQuery = searchParams.get('search') || ''

  // Use search if there's a search query, otherwise use list
  const listQuery = useTicketList(
    { status: statusFilter, type: typeFilter },
    { enabled: !searchQuery }
  )
  const searchQueryResult = useTicketSearch(searchQuery, {
    enabled: !!searchQuery,
  })

  const isSearching = !!searchQuery
  const tickets = isSearching
    ? searchQueryResult.data?.tickets
    : listQuery.data?.tickets
  const isLoading = isSearching ? searchQueryResult.isLoading : listQuery.isLoading
  const error = isSearching ? searchQueryResult.error : listQuery.error

  const handleFilterChange = (key: string, value: string) => {
    const newParams = new URLSearchParams(searchParams)
    if (value) {
      newParams.set(key, value)
    } else {
      newParams.delete(key)
    }
    // Clear search when using filters
    newParams.delete('search')
    setSearchParams(newParams)
  }

  const clearFilters = () => {
    setSearchParams({})
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">
            {isSearching ? `Search: "${searchQuery}"` : 'Tickets'}
          </h1>
          <p className="text-muted-foreground">
            {tickets?.length ?? 0} ticket{tickets?.length !== 1 ? 's' : ''}{' '}
            {isSearching ? 'found' : ''}
          </p>
        </div>
        <Link to="/tickets/new">
          <Button>
            <Plus className="h-4 w-4 mr-2" />
            New Ticket
          </Button>
        </Link>
      </div>

      {!isSearching && (
        <div className="flex items-center gap-4">
          <Dropdown
            value={statusFilter}
            onChange={(v) => handleFilterChange('status', v)}
            className="w-40"
            options={[
              { value: '', label: 'All Statuses' },
              { value: 'open', label: 'Open' },
              { value: 'in_progress', label: 'In Progress' },
              { value: 'closed', label: 'Closed' },
              { value: 'blocked', label: 'Blocked' },
            ]}
          />

          <Dropdown
            value={typeFilter}
            onChange={(v) => handleFilterChange('type', v)}
            className="w-40"
            options={[
              { value: '', label: 'All Types' },
              { value: 'bug', label: 'Bug' },
              { value: 'feature', label: 'Feature' },
              { value: 'task', label: 'Task' },
              { value: 'epic', label: 'Epic' },
            ]}
          />

          {(statusFilter || typeFilter) && (
            <Button variant="ghost" size="sm" onClick={clearFilters}>
              Clear filters
            </Button>
          )}
        </div>
      )}

      {isSearching && (
        <Button variant="ghost" size="sm" onClick={clearFilters}>
          Clear search
        </Button>
      )}

      <TicketList
        tickets={tickets}
        isLoading={isLoading}
        error={error as Error | null}
        emptyMessage={
          isSearching
            ? 'No tickets match your search'
            : 'No tickets found. Create one to get started!'
        }
      />
    </div>
  )
}
