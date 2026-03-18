import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Play, CheckCircle, XCircle, Pause, Clock, Coins, Hash, RefreshCw } from 'lucide-react'
import { listRuns, subscribeToLive, type ListRunsParams } from '@/lib/api'
import { formatDuration, formatCost, formatTokens, formatRelativeTime, getStatusColor, cn } from '@/lib/utils'
import type { Run, RunStatus } from '@/types'

const statusIcons: Record<RunStatus, typeof Play> = {
  running: Play,
  completed: CheckCircle,
  failed: XCircle,
  cancelled: Pause,
}

export default function RunList() {
  const [filters, setFilters] = useState<ListRunsParams>({
    limit: 50,
  })

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['runs', filters],
    queryFn: () => listRuns(filters),
    refetchInterval: filters.status === 'running' ? 5000 : false,
  })

  // Subscribe to live updates
  useEffect(() => {
    const source = subscribeToLive((event) => {
      try {
        const data = JSON.parse(event.data)
        if (data.event === 'run_update') {
          refetch()
        }
      } catch {
        // Ignore parse errors
      }
    })

    return () => {
      source.close()
    }
  }, [refetch])

  const runs = data?.data ?? []

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Workflow Runs</h1>
        <button
          onClick={() => refetch()}
          className="btn btn-secondary"
          disabled={isLoading}
        >
          <RefreshCw className={cn('h-4 w-4 mr-2', isLoading && 'animate-spin')} />
          Refresh
        </button>
      </div>

      {/* Filters */}
      <div className="card p-4 mb-6">
        <div className="flex flex-wrap gap-4">
          <div>
            <label className="label">Status</label>
            <select
              className="select"
              value={filters.status ?? ''}
              onChange={(e) => setFilters({ ...filters, status: e.target.value || undefined })}
            >
              <option value="">All</option>
              <option value="running">Running</option>
              <option value="completed">Completed</option>
              <option value="failed">Failed</option>
              <option value="cancelled">Cancelled</option>
            </select>
          </div>

          <div>
            <label className="label">Workflow</label>
            <input
              type="text"
              className="input"
              placeholder="Filter by workflow..."
              value={filters.workflow ?? ''}
              onChange={(e) => setFilters({ ...filters, workflow: e.target.value || undefined })}
            />
          </div>

          <div>
            <label className="label">Time Range</label>
            <select
              className="select"
              value={filters.since ?? ''}
              onChange={(e) => setFilters({ ...filters, since: e.target.value || undefined })}
            >
              <option value="">All time</option>
              <option value="1h">Last hour</option>
              <option value="24h">Last 24 hours</option>
              <option value="7d">Last 7 days</option>
              <option value="30d">Last 30 days</option>
            </select>
          </div>
        </div>
      </div>

      {/* Error state */}
      {error && (
        <div className="card p-4 bg-red-50 border-red-200 mb-6">
          <p className="text-red-700">Error loading runs: {(error as Error).message}</p>
        </div>
      )}

      {/* Runs table */}
      <div className="card overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Status
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Workflow
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Run ID
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Duration
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Tokens
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Cost
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Started
              </th>
            </tr>
          </thead>
          <tbody className="bg-white divide-y divide-gray-200">
            {isLoading && runs.length === 0 ? (
              <tr>
                <td colSpan={7} className="px-6 py-12 text-center text-gray-500">
                  Loading...
                </td>
              </tr>
            ) : runs.length === 0 ? (
              <tr>
                <td colSpan={7} className="px-6 py-12 text-center text-gray-500">
                  No runs found
                </td>
              </tr>
            ) : (
              runs.map((run) => <RunRow key={run.id} run={run} />)
            )}
          </tbody>
        </table>
      </div>

      {/* Load more */}
      {data?.has_more && (
        <div className="mt-4 text-center">
          <button
            className="btn btn-secondary"
            onClick={() => setFilters({ ...filters, cursor: data.cursor })}
          >
            Load more
          </button>
        </div>
      )}
    </div>
  )
}

function RunRow({ run }: { run: Run }) {
  const StatusIcon = statusIcons[run.status]

  return (
    <tr className="hover:bg-gray-50">
      <td className="px-6 py-4 whitespace-nowrap">
        <span className={cn('badge', getStatusColor(run.status))}>
          <StatusIcon className="h-3 w-3 mr-1" />
          {run.status}
        </span>
      </td>
      <td className="px-6 py-4 whitespace-nowrap">
        <span className="text-sm font-medium text-gray-900">{run.workflow_name}</span>
      </td>
      <td className="px-6 py-4 whitespace-nowrap">
        <Link
          to={`/runs/${run.id}`}
          className="text-sm text-primary-600 hover:text-primary-800 font-mono"
        >
          {run.id.slice(0, 12)}...
        </Link>
      </td>
      <td className="px-6 py-4 whitespace-nowrap">
        <span className="flex items-center text-sm text-gray-600">
          <Clock className="h-4 w-4 mr-1" />
          {run.status === 'running' ? 'Running...' : formatDuration(run.duration_ms)}
        </span>
      </td>
      <td className="px-6 py-4 whitespace-nowrap">
        <span className="flex items-center text-sm text-gray-600">
          <Hash className="h-4 w-4 mr-1" />
          {formatTokens(run.total_tokens.total_tokens)}
        </span>
      </td>
      <td className="px-6 py-4 whitespace-nowrap">
        <span className="flex items-center text-sm text-gray-600">
          <Coins className="h-4 w-4 mr-1" />
          {formatCost(run.estimated_cost.total)}
        </span>
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
        {formatRelativeTime(run.started_at)}
      </td>
    </tr>
  )
}
