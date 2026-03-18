import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, Play, CheckCircle, XCircle, Clock, Coins, Hash, RefreshCw, GitCompare } from 'lucide-react'
import { getRun, getSpans } from '@/lib/api'
import { formatDuration, formatCost, formatTokens, formatDate, getStatusColor, cn } from '@/lib/utils'
import Timeline from '@/components/Timeline'
import GraphView from '@/components/GraphView'
import PromptInspector from '@/components/PromptInspector'
import ReplayConsole from '@/components/ReplayConsole'
import type { Span } from '@/types'

type Tab = 'timeline' | 'graph' | 'replay'

export default function RunDetail() {
  const { runId } = useParams<{ runId: string }>()
  const [activeTab, setActiveTab] = useState<Tab>('timeline')
  const [selectedSpan, setSelectedSpan] = useState<Span | null>(null)

  const { data: run, isLoading: runLoading, error: runError } = useQuery({
    queryKey: ['run', runId],
    queryFn: () => getRun(runId!),
    enabled: !!runId,
  })

  const { data: spans, isLoading: spansLoading } = useQuery({
    queryKey: ['spans', runId],
    queryFn: () => getSpans(runId!),
    enabled: !!runId,
  })

  if (runLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="h-8 w-8 animate-spin text-gray-400" />
      </div>
    )
  }

  if (runError || !run) {
    return (
      <div className="card p-8 text-center">
        <XCircle className="h-12 w-12 mx-auto text-red-400 mb-4" />
        <h2 className="text-xl font-semibold text-gray-900 mb-2">Run not found</h2>
        <p className="text-gray-600 mb-4">
          {runError ? (runError as Error).message : 'The requested run could not be found.'}
        </p>
        <Link to="/" className="btn btn-primary">
          Back to runs
        </Link>
      </div>
    )
  }

  const StatusIcon = run.status === 'completed' ? CheckCircle :
                      run.status === 'failed' ? XCircle : Play

  return (
    <div>
      {/* Header */}
      <div className="mb-6">
        <Link to="/" className="flex items-center text-gray-600 hover:text-gray-900 mb-4">
          <ArrowLeft className="h-4 w-4 mr-2" />
          Back to runs
        </Link>

        <div className="flex items-start justify-between">
          <div>
            <h1 className="text-2xl font-bold text-gray-900 mb-2">
              {run.workflow_name}
            </h1>
            <p className="text-sm text-gray-500 font-mono">{run.id}</p>
          </div>

          <div className="flex items-center gap-3">
            <Link
              to={`/diff?base=${run.id}`}
              className="btn btn-secondary"
            >
              <GitCompare className="h-4 w-4 mr-2" />
              Compare
            </Link>
          </div>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-5 gap-4 mb-6">
        <div className="card p-4">
          <div className="text-sm text-gray-500 mb-1">Status</div>
          <span className={cn('badge', getStatusColor(run.status))}>
            <StatusIcon className="h-3 w-3 mr-1" />
            {run.status}
          </span>
        </div>

        <div className="card p-4">
          <div className="text-sm text-gray-500 mb-1">Duration</div>
          <div className="flex items-center text-lg font-semibold">
            <Clock className="h-5 w-5 mr-2 text-gray-400" />
            {run.status === 'running' ? 'Running...' : formatDuration(run.duration_ms)}
          </div>
        </div>

        <div className="card p-4">
          <div className="text-sm text-gray-500 mb-1">Tokens</div>
          <div className="flex items-center text-lg font-semibold">
            <Hash className="h-5 w-5 mr-2 text-gray-400" />
            {formatTokens(run.total_tokens.total_tokens)}
          </div>
          <div className="text-xs text-gray-400 mt-1">
            {formatTokens(run.total_tokens.input_tokens)} in / {formatTokens(run.total_tokens.output_tokens)} out
          </div>
        </div>

        <div className="card p-4">
          <div className="text-sm text-gray-500 mb-1">Cost</div>
          <div className="flex items-center text-lg font-semibold">
            <Coins className="h-5 w-5 mr-2 text-gray-400" />
            {formatCost(run.estimated_cost.total)}
          </div>
        </div>

        <div className="card p-4">
          <div className="text-sm text-gray-500 mb-1">Started</div>
          <div className="text-sm font-medium">
            {formatDate(run.started_at)}
          </div>
          {run.completed_at && (
            <div className="text-xs text-gray-400 mt-1">
              Completed: {formatDate(run.completed_at)}
            </div>
          )}
        </div>
      </div>

      {/* Tabs */}
      <div className="border-b border-gray-200 mb-6">
        <nav className="flex gap-4">
          {(['timeline', 'graph', 'replay'] as Tab[]).map((tab) => (
            <button
              key={tab}
              onClick={() => setActiveTab(tab)}
              className={cn(
                'px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors',
                activeTab === tab
                  ? 'border-primary-500 text-primary-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700'
              )}
            >
              {tab.charAt(0).toUpperCase() + tab.slice(1)}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab content */}
      <div className="flex gap-6">
        <div className={cn('flex-1', selectedSpan && 'w-2/3')}>
          {activeTab === 'timeline' && (
            <Timeline
              spans={spans ?? []}
              isLoading={spansLoading}
              onSelectSpan={setSelectedSpan}
              selectedSpanId={selectedSpan?.id}
              runStartTime={new Date(run.started_at).getTime()}
              runDuration={run.duration_ms}
            />
          )}

          {activeTab === 'graph' && (
            <GraphView
              spans={spans ?? []}
              onSelectSpan={setSelectedSpan}
              selectedSpanId={selectedSpan?.id}
            />
          )}

          {activeTab === 'replay' && (
            <ReplayConsole runId={run.id} />
          )}
        </div>

        {/* Prompt Inspector panel */}
        {selectedSpan && selectedSpan.kind === 'llm' && (
          <div className="w-1/3">
            <PromptInspector
              runId={run.id}
              span={selectedSpan}
              onClose={() => setSelectedSpan(null)}
            />
          </div>
        )}
      </div>
    </div>
  )
}
