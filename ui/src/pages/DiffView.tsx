import { useState, useEffect } from 'react'
import { useParams, useSearchParams, Link } from 'react-router-dom'
import { useQuery, useMutation } from '@tanstack/react-query'
import {
  ArrowLeft,
  RefreshCw,
  CheckCircle,
  XCircle,
  AlertTriangle,
  Plus,
  Minus,
  Equal,
  GitCompare,
  Clock,
  Hash,
  Coins,
  ChevronDown,
  ChevronRight,
} from 'lucide-react'
import { computeDiff, getDiffByRuns, listRuns, type ComputeDiffParams } from '@/lib/api'
import { formatDuration, formatCost, formatTokens, cn } from '@/lib/utils'
import type { RunDiff, NodeDiff } from '@/types'

export default function DiffView() {
  const { baseId, compareId } = useParams<{ baseId?: string; compareId?: string }>()
  const [searchParams] = useSearchParams()
  const queryBaseId = searchParams.get('base') ?? baseId
  const queryCompareId = searchParams.get('compare') ?? compareId

  const [baseRunId, setBaseRunId] = useState(queryBaseId ?? '')
  const [compareRunId, setCompareRunId] = useState(queryCompareId ?? '')
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(new Set())

  // Fetch available runs for selection
  const { data: runsData } = useQuery({
    queryKey: ['runs', { limit: 100 }],
    queryFn: () => listRuns({ limit: 100 }),
  })

  // Try to get cached diff first
  const { data: cachedDiff, isLoading: cacheLoading } = useQuery({
    queryKey: ['diff-cached', baseRunId, compareRunId],
    queryFn: () => getDiffByRuns(baseRunId, compareRunId),
    enabled: !!(baseRunId && compareRunId),
  })

  // Compute diff mutation
  const { mutate: compute, data: computedDiff, isPending: computing } = useMutation({
    mutationFn: (params: ComputeDiffParams) => computeDiff(params),
  })

  // Use cached diff if available, otherwise computed diff
  const diff = cachedDiff ?? computedDiff

  // Auto-compute diff if not cached
  useEffect(() => {
    if (baseRunId && compareRunId && !cacheLoading && !cachedDiff && !computedDiff && !computing) {
      compute({
        base_run_id: baseRunId,
        compare_run_id: compareRunId,
        include_content: true,
      })
    }
  }, [baseRunId, compareRunId, cacheLoading, cachedDiff, computedDiff, computing, compute])

  const runs = runsData?.data ?? []
  const isLoading = cacheLoading || computing

  const toggleNode = (nodeId: string) => {
    const newExpanded = new Set(expandedNodes)
    if (newExpanded.has(nodeId)) {
      newExpanded.delete(nodeId)
    } else {
      newExpanded.add(nodeId)
    }
    setExpandedNodes(newExpanded)
  }

  return (
    <div>
      {/* Header */}
      <div className="mb-6">
        <Link to="/" className="flex items-center text-gray-600 hover:text-gray-900 mb-4">
          <ArrowLeft className="h-4 w-4 mr-2" />
          Back to runs
        </Link>
        <h1 className="text-2xl font-bold text-gray-900">Run Comparison</h1>
      </div>

      {/* Run selection */}
      <div className="card p-4 mb-6">
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="label">Base Run</label>
            <select
              className="select w-full"
              value={baseRunId}
              onChange={(e) => setBaseRunId(e.target.value)}
            >
              <option value="">Select a run...</option>
              {runs.map((run) => (
                <option key={run.id} value={run.id}>
                  {run.workflow_name} - {run.id.slice(0, 12)}... ({run.status})
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="label">Compare Run</label>
            <select
              className="select w-full"
              value={compareRunId}
              onChange={(e) => setCompareRunId(e.target.value)}
            >
              <option value="">Select a run...</option>
              {runs
                .filter((run) => run.id !== baseRunId)
                .map((run) => (
                  <option key={run.id} value={run.id}>
                    {run.workflow_name} - {run.id.slice(0, 12)}... ({run.status})
                  </option>
                ))}
            </select>
          </div>
        </div>

        <div className="mt-4 flex justify-end">
          <button
            onClick={() => {
              if (baseRunId && compareRunId) {
                compute({
                  base_run_id: baseRunId,
                  compare_run_id: compareRunId,
                  include_content: true,
                })
              }
            }}
            className="btn btn-primary"
            disabled={!baseRunId || !compareRunId || isLoading}
          >
            {isLoading ? (
              <>
                <RefreshCw className="h-4 w-4 mr-2 animate-spin" />
                Computing...
              </>
            ) : (
              <>
                <GitCompare className="h-4 w-4 mr-2" />
                Compare Runs
              </>
            )}
          </button>
        </div>
      </div>

      {/* Diff results */}
      {diff && (
        <>
          {/* Summary */}
          <DiffSummaryCard diff={diff} />

          {/* Node diffs */}
          <div className="card mt-6 overflow-hidden">
            <div className="px-6 py-4 border-b border-gray-200">
              <h3 className="font-semibold text-gray-900">Node Differences</h3>
              <p className="text-sm text-gray-500 mt-1">
                {diff.node_diffs.length} node{diff.node_diffs.length !== 1 && 's'} compared
              </p>
            </div>

            <div className="divide-y divide-gray-100">
              {diff.node_diffs.map((nodeDiff) => (
                <NodeDiffRow
                  key={nodeDiff.node_id}
                  nodeDiff={nodeDiff}
                  expanded={expandedNodes.has(nodeDiff.node_id)}
                  onToggle={() => toggleNode(nodeDiff.node_id)}
                />
              ))}

              {diff.node_diffs.length === 0 && (
                <div className="px-6 py-8 text-center text-gray-500">
                  No node differences found
                </div>
              )}
            </div>
          </div>

          {/* Cost comparison */}
          <CostComparisonCard diff={diff} />
        </>
      )}

      {/* Empty state */}
      {!diff && !isLoading && baseRunId && compareRunId && (
        <div className="card p-8 text-center">
          <GitCompare className="h-12 w-12 mx-auto text-gray-400 mb-4" />
          <h2 className="text-xl font-semibold text-gray-900 mb-2">Ready to Compare</h2>
          <p className="text-gray-600">
            Click "Compare Runs" to compute the difference between the selected runs.
          </p>
        </div>
      )}
    </div>
  )
}

function DiffSummaryCard({ diff }: { diff: RunDiff }) {
  const { summary } = diff

  return (
    <div className="card p-4">
      <h3 className="font-semibold text-gray-900 mb-4">Summary</h3>

      <div className="grid grid-cols-5 gap-4">
        <div className="text-center">
          <div className="text-sm text-gray-500 mb-1">Status Match</div>
          {summary.status_match ? (
            <CheckCircle className="h-8 w-8 mx-auto text-green-500" />
          ) : (
            <XCircle className="h-8 w-8 mx-auto text-red-500" />
          )}
        </div>

        <div className="text-center">
          <div className="text-sm text-gray-500 mb-1">Duration Delta</div>
          <div className={cn(
            'text-lg font-semibold flex items-center justify-center gap-1',
            summary.duration_delta_ms > 0 ? 'text-red-600' : summary.duration_delta_ms < 0 ? 'text-green-600' : 'text-gray-600'
          )}>
            <Clock className="h-4 w-4" />
            {summary.duration_delta_ms > 0 ? '+' : ''}{formatDuration(Math.abs(summary.duration_delta_ms))}
          </div>
        </div>

        <div className="text-center">
          <div className="text-sm text-gray-500 mb-1">Token Delta</div>
          <div className={cn(
            'text-lg font-semibold flex items-center justify-center gap-1',
            summary.token_delta > 0 ? 'text-red-600' : summary.token_delta < 0 ? 'text-green-600' : 'text-gray-600'
          )}>
            <Hash className="h-4 w-4" />
            {summary.token_delta > 0 ? '+' : ''}{formatTokens(summary.token_delta)}
          </div>
        </div>

        <div className="text-center">
          <div className="text-sm text-gray-500 mb-1">Cost Delta</div>
          <div className={cn(
            'text-lg font-semibold flex items-center justify-center gap-1',
            summary.cost_delta > 0 ? 'text-red-600' : summary.cost_delta < 0 ? 'text-green-600' : 'text-gray-600'
          )}>
            <Coins className="h-4 w-4" />
            {summary.cost_delta > 0 ? '+' : ''}{formatCost(summary.cost_delta)}
          </div>
        </div>

        <div className="text-center">
          <div className="text-sm text-gray-500 mb-1">Path Divergence</div>
          {summary.path_divergence ? (
            <AlertTriangle className="h-8 w-8 mx-auto text-yellow-500" />
          ) : (
            <Equal className="h-8 w-8 mx-auto text-green-500" />
          )}
        </div>
      </div>
    </div>
  )
}

function NodeDiffRow({
  nodeDiff,
  expanded,
  onToggle,
}: {
  nodeDiff: NodeDiff
  expanded: boolean
  onToggle: () => void
}) {
  const statusIcon = nodeDiff.status === 'added' ? Plus :
                     nodeDiff.status === 'removed' ? Minus : Equal

  const statusColor = nodeDiff.status === 'added' ? 'text-green-600' :
                      nodeDiff.status === 'removed' ? 'text-red-600' : 'text-blue-600'

  const durationDelta = nodeDiff.duration_compare_ms - nodeDiff.duration_base_ms

  return (
    <div>
      <div
        className="px-6 py-4 flex items-center justify-between cursor-pointer hover:bg-gray-50"
        onClick={onToggle}
      >
        <div className="flex items-center gap-3">
          {expanded ? (
            <ChevronDown className="h-4 w-4 text-gray-400" />
          ) : (
            <ChevronRight className="h-4 w-4 text-gray-400" />
          )}

          {/* Status icon */}
          {statusIcon === Plus ? (
            <Plus className={cn('h-5 w-5', statusColor)} />
          ) : statusIcon === Minus ? (
            <Minus className={cn('h-5 w-5', statusColor)} />
          ) : (
            <Equal className={cn('h-5 w-5', statusColor)} />
          )}

          <div>
            <span className="font-medium text-gray-900">{nodeDiff.node_id}</span>
            <span className="text-sm text-gray-500 ml-2">({nodeDiff.node_type})</span>
          </div>
        </div>

        <div className="flex items-center gap-6 text-sm">
          {/* Duration comparison */}
          <div className="text-gray-600">
            {formatDuration(nodeDiff.duration_base_ms)} → {formatDuration(nodeDiff.duration_compare_ms)}
            <span className={cn(
              'ml-2',
              durationDelta > 0 ? 'text-red-600' : durationDelta < 0 ? 'text-green-600' : 'text-gray-400'
            )}>
              ({durationDelta > 0 ? '+' : ''}{formatDuration(durationDelta)})
            </span>
          </div>

          {/* Token comparison */}
          {nodeDiff.token_diff && (
            <div className="text-gray-600">
              <Hash className="h-3 w-3 inline mr-1" />
              {formatTokens(nodeDiff.token_diff.base_tokens.total_tokens)} →{' '}
              {formatTokens(nodeDiff.token_diff.compare_tokens.total_tokens)}
              <span className={cn(
                'ml-2',
                nodeDiff.token_diff.delta > 0 ? 'text-red-600' : nodeDiff.token_diff.delta < 0 ? 'text-green-600' : 'text-gray-400'
              )}>
                ({nodeDiff.token_diff.delta > 0 ? '+' : ''}{formatTokens(nodeDiff.token_diff.delta)})
              </span>
            </div>
          )}
        </div>
      </div>

      {/* Expanded content diff */}
      {expanded && nodeDiff.output_diff && (
        <div className="px-6 pb-4">
          <div className="grid grid-cols-2 gap-4">
            {/* Base output */}
            <div>
              <div className="text-xs font-medium text-gray-500 uppercase mb-2">Base Output</div>
              <pre className="bg-red-50 border border-red-200 rounded-md p-3 text-sm text-gray-800 whitespace-pre-wrap overflow-auto max-h-64">
                {nodeDiff.output_diff.base_text || '(empty)'}
              </pre>
            </div>

            {/* Compare output */}
            <div>
              <div className="text-xs font-medium text-gray-500 uppercase mb-2">Compare Output</div>
              <pre className="bg-green-50 border border-green-200 rounded-md p-3 text-sm text-gray-800 whitespace-pre-wrap overflow-auto max-h-64">
                {nodeDiff.output_diff.compare_text || '(empty)'}
              </pre>
            </div>
          </div>

          {/* Similarity score */}
          {nodeDiff.output_diff.similarity !== undefined && (
            <div className="mt-3 text-sm text-gray-500">
              Similarity: {(nodeDiff.output_diff.similarity * 100).toFixed(1)}%
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function CostComparisonCard({ diff }: { diff: RunDiff }) {
  const { cost_diff } = diff

  return (
    <div className="card mt-6 p-4">
      <h3 className="font-semibold text-gray-900 mb-4">Cost Comparison</h3>

      <div className="grid grid-cols-3 gap-6">
        <div>
          <div className="text-sm text-gray-500 mb-1">Base Run Cost</div>
          <div className="text-2xl font-bold text-gray-900">{formatCost(cost_diff.base_cost)}</div>
        </div>

        <div>
          <div className="text-sm text-gray-500 mb-1">Compare Run Cost</div>
          <div className="text-2xl font-bold text-gray-900">{formatCost(cost_diff.compare_cost)}</div>
        </div>

        <div>
          <div className="text-sm text-gray-500 mb-1">Difference</div>
          <div className={cn(
            'text-2xl font-bold',
            cost_diff.delta > 0 ? 'text-red-600' : cost_diff.delta < 0 ? 'text-green-600' : 'text-gray-600'
          )}>
            {cost_diff.delta > 0 ? '+' : ''}{formatCost(cost_diff.delta)}
          </div>
        </div>
      </div>

      {/* Provider breakdown */}
      {cost_diff.by_provider && Object.keys(cost_diff.by_provider).length > 0 && (
        <div className="mt-6">
          <div className="text-sm font-medium text-gray-500 mb-2">By Provider</div>
          <div className="space-y-2">
            {Object.entries(cost_diff.by_provider).map(([provider, delta]) => (
              <div key={provider} className="flex items-center justify-between">
                <span className="text-sm text-gray-700">{provider}</span>
                <span className={cn(
                  'text-sm font-medium',
                  delta > 0 ? 'text-red-600' : delta < 0 ? 'text-green-600' : 'text-gray-600'
                )}>
                  {delta > 0 ? '+' : ''}{formatCost(delta)}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Model breakdown */}
      {cost_diff.by_model && Object.keys(cost_diff.by_model).length > 0 && (
        <div className="mt-4">
          <div className="text-sm font-medium text-gray-500 mb-2">By Model</div>
          <div className="space-y-2">
            {Object.entries(cost_diff.by_model).map(([model, delta]) => (
              <div key={model} className="flex items-center justify-between">
                <span className="text-sm text-gray-700 font-mono">{model}</span>
                <span className={cn(
                  'text-sm font-medium',
                  delta > 0 ? 'text-red-600' : delta < 0 ? 'text-green-600' : 'text-gray-600'
                )}>
                  {delta > 0 ? '+' : ''}{formatCost(delta)}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
