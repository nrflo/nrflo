import { useSearchParams, Link } from 'react-router-dom'
import { Plus, ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { TicketList } from '@/components/tickets/TicketList'
import { useTicketList, useTicketSearch } from '@/hooks/useTickets'

const SORT_OPTIONS = [
  { value: 'updated_at', label: 'Updated' },
  { value: 'created_at', label: 'Created' },
  { value: 'priority', label: 'Priority' },
  { value: 'title', label: 'Title' },
  { value: 'status', label: 'Status' },
]

const PER_PAGE = 30

export function TicketListPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const statusFilter = searchParams.get('status') || ''
  const typeFilter = searchParams.get('type') || ''
  const searchQuery = searchParams.get('search') || ''
  const page = parseInt(searchParams.get('page') || '1', 10) || 1
  const sortBy = searchParams.get('sort_by') || 'updated_at'
  const sortOrder = searchParams.get('sort_order') || 'desc'

  const listQuery = useTicketList(
    { status: statusFilter, type: typeFilter, page, per_page: PER_PAGE, sort_by: sortBy, sort_order: sortOrder },
    { enabled: !searchQuery, placeholderData: (prev) => prev }
  )
  const searchQueryResult = useTicketSearch(searchQuery, {
    enabled: !!searchQuery,
  })

  const isSearching = !!searchQuery
  const data = isSearching ? searchQueryResult.data : listQuery.data
  const tickets = data?.tickets
  const totalCount = isSearching ? (tickets?.length ?? 0) : (listQuery.data?.total_count ?? 0)
  const totalPages = isSearching ? 1 : (listQuery.data?.total_pages ?? 1)
  const isLoading = isSearching ? searchQueryResult.isLoading : listQuery.isLoading
  const error = isSearching ? searchQueryResult.error : listQuery.error

  const handleFilterChange = (key: string, value: string) => {
    const newParams = new URLSearchParams(searchParams)
    if (value) {
      newParams.set(key, value)
    } else {
      newParams.delete(key)
    }
    newParams.delete('search')
    newParams.delete('page')
    setSearchParams(newParams)
  }

  const handleSortChange = (key: string, value: string) => {
    const newParams = new URLSearchParams(searchParams)
    const defaultValue = key === 'sort_by' ? 'updated_at' : 'desc'
    if (value && value !== defaultValue) {
      newParams.set(key, value)
    } else {
      newParams.delete(key)
    }
    newParams.delete('page')
    setSearchParams(newParams)
  }

  const handlePageChange = (newPage: number) => {
    const newParams = new URLSearchParams(searchParams)
    if (newPage > 1) {
      newParams.set('page', String(newPage))
    } else {
      newParams.delete('page')
    }
    setSearchParams(newParams)
  }

  const clearFilters = () => {
    setSearchParams({})
  }

  return (
    <div className="max-w-7xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">
            {isSearching ? `Search: "${searchQuery}"` : 'Tickets'}
          </h1>
          <p className="text-muted-foreground">
            {totalCount} ticket{totalCount !== 1 ? 's' : ''}{' '}
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

          <Dropdown
            value={sortBy}
            onChange={(v) => handleSortChange('sort_by', v)}
            className="w-36"
            options={SORT_OPTIONS}
          />

          <Dropdown
            value={sortOrder}
            onChange={(v) => handleSortChange('sort_order', v)}
            className="w-32"
            options={[
              { value: 'desc', label: 'Newest' },
              { value: 'asc', label: 'Oldest' },
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

      {!isSearching && totalPages > 1 && (
        <div className="flex items-center justify-center gap-4">
          <Button
            variant="outline"
            size="sm"
            onClick={() => handlePageChange(page - 1)}
            disabled={page <= 1}
          >
            <ChevronLeft className="h-4 w-4 mr-1" />
            Previous
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {page} of {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => handlePageChange(page + 1)}
            disabled={page >= totalPages}
          >
            Next
            <ChevronRight className="h-4 w-4 ml-1" />
          </Button>
        </div>
      )}
    </div>
  )
}
