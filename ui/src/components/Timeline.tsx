import { useMemo } from 'react'
import { RefreshCw, MessageSquare, Wrench, ArrowRight } from 'lucide-react'
import { formatDuration, formatTokens, getStatusDotColor, cn } from '@/lib/utils'
import type { Span, SpanKind } from '@/types'

interface TimelineProps {
  spans: Span[]
  isLoading: boolean
  onSelectSpan: (span: Span | null) => void
  selectedSpanId?: string
  runStartTime: number
  runDuration: number
}

const kindIcons: Record<SpanKind, typeof MessageSquare> = {
  node: RefreshCw,
  llm: MessageSquare,
  tool: Wrench,
  edge: ArrowRight,
  custom: RefreshCw,
}

const kindColors: Record<SpanKind, string> = {
  node: 'bg-blue-500',
  llm: 'bg-purple-500',
  tool: 'bg-orange-500',
  edge: 'bg-gray-400',
  custom: 'bg-gray-500',
}

export default function Timeline({
  spans,
  isLoading,
  onSelectSpan,
  selectedSpanId,
  runStartTime,
  runDuration,
}: TimelineProps) {
  // Group spans by node
  const nodeSpans = useMemo(() => {
    const nodes: Map<string, Span[]> = new Map()

    for (const span of spans) {
      if (span.kind === 'node') {
        const nodeId = span.node?.node_id ?? span.name
        if (!nodes.has(nodeId)) {
          nodes.set(nodeId, [])
        }
        nodes.get(nodeId)!.push(span)
      }
    }

    // Add child spans (LLM, tool) to their parent nodes
    for (const span of spans) {
      if (span.kind !== 'node' && span.parent_id) {
        const parentSpan = spans.find(s => s.id === span.parent_id)
        if (parentSpan?.kind === 'node') {
          const nodeId = parentSpan.node?.node_id ?? parentSpan.name
          if (nodes.has(nodeId)) {
            nodes.get(nodeId)!.push(span)
          }
        }
      }
    }

    return nodes
  }, [spans])

  // Calculate time scale
  const totalDuration = runDuration || 1000
  const pixelsPerMs = 800 / totalDuration

  if (isLoading) {
    return (
      <div className="card p-8 flex items-center justify-center">
        <RefreshCw className="h-8 w-8 animate-spin text-gray-400" />
      </div>
    )
  }

  if (spans.length === 0) {
    return (
      <div className="card p-8 text-center text-gray-500">
        No spans found for this run
      </div>
    )
  }

  return (
    <div className="card overflow-hidden">
      {/* Time axis */}
      <div className="px-6 py-2 bg-gray-50 border-b border-gray-200">
        <div className="flex justify-between text-xs text-gray-500">
          <span>0s</span>
          <span>{formatDuration(totalDuration / 4)}</span>
          <span>{formatDuration(totalDuration / 2)}</span>
          <span>{formatDuration(totalDuration * 0.75)}</span>
          <span>{formatDuration(totalDuration)}</span>
        </div>
      </div>

      {/* Swimlanes */}
      <div className="divide-y divide-gray-100">
        {Array.from(nodeSpans.entries()).map(([nodeId, nodeGroupSpans]) => {
          const nodeSpan = nodeGroupSpans.find(s => s.kind === 'node')
          const childSpans = nodeGroupSpans.filter(s => s.kind !== 'node')

          if (!nodeSpan) return null

          const startOffset = new Date(nodeSpan.started_at).getTime() - runStartTime
          const left = Math.max(0, startOffset * pixelsPerMs)
          const width = Math.max(4, nodeSpan.duration_ms * pixelsPerMs)

          return (
            <div key={nodeId} className="px-6 py-3">
              {/* Node header */}
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-2">
                  <span className={cn('w-2 h-2 rounded-full', getStatusDotColor(nodeSpan.status))} />
                  <span className="text-sm font-medium text-gray-900">{nodeId}</span>
                  <span className="text-xs text-gray-500">
                    ({nodeSpan.node?.node_type ?? 'node'})
                  </span>
                </div>
                <div className="text-xs text-gray-500">
                  {formatDuration(nodeSpan.duration_ms)}
                </div>
              </div>

              {/* Timeline bar */}
              <div className="relative h-6">
                <div
                  className={cn(
                    'absolute h-full rounded cursor-pointer transition-opacity',
                    kindColors.node,
                    nodeSpan.status === 'error' && 'bg-red-500',
                    selectedSpanId === nodeSpan.id ? 'opacity-100 ring-2 ring-primary-400' : 'opacity-70 hover:opacity-100'
                  )}
                  style={{ left: `${left}px`, width: `${width}px` }}
                  onClick={() => onSelectSpan(nodeSpan)}
                  title={`${nodeId}: ${formatDuration(nodeSpan.duration_ms)}`}
                />

                {/* Child spans (LLM, tool) */}
                {childSpans.map((childSpan) => {
                  const childStart = new Date(childSpan.started_at).getTime() - runStartTime
                  const childLeft = Math.max(0, childStart * pixelsPerMs)
                  const childWidth = Math.max(2, childSpan.duration_ms * pixelsPerMs)
                  const Icon = kindIcons[childSpan.kind]

                  return (
                    <div
                      key={childSpan.id}
                      className={cn(
                        'absolute h-4 top-1 rounded cursor-pointer transition-opacity flex items-center justify-center',
                        kindColors[childSpan.kind],
                        childSpan.status === 'error' && 'bg-red-500',
                        selectedSpanId === childSpan.id ? 'opacity-100 ring-2 ring-primary-400' : 'opacity-80 hover:opacity-100'
                      )}
                      style={{ left: `${childLeft}px`, width: `${Math.max(childWidth, 16)}px` }}
                      onClick={(e) => {
                        e.stopPropagation()
                        onSelectSpan(childSpan)
                      }}
                      title={`${childSpan.name}: ${formatDuration(childSpan.duration_ms)}`}
                    >
                      {childWidth > 16 && <Icon className="h-3 w-3 text-white" />}
                    </div>
                  )
                })}
              </div>

              {/* Token breakdown for LLM spans */}
              {childSpans.some(s => s.kind === 'llm') && (
                <div className="flex gap-2 mt-2">
                  {childSpans
                    .filter(s => s.kind === 'llm' && s.llm)
                    .map((llmSpan) => (
                      <div key={llmSpan.id} className="text-xs text-gray-500">
                        <span className="font-medium">{llmSpan.llm?.model}:</span>{' '}
                        {formatTokens(llmSpan.llm?.tokens.total_tokens ?? 0)} tokens
                      </div>
                    ))}
                </div>
              )}
            </div>
          )
        })}
      </div>

      {/* Token breakdown summary */}
      <div className="px-6 py-3 bg-gray-50 border-t border-gray-200">
        <div className="text-xs text-gray-500 mb-2">Token Distribution</div>
        <div className="flex h-3 rounded-full overflow-hidden bg-gray-200">
          {Array.from(nodeSpans.entries()).map(([nodeId, nodeGroupSpans]) => {
            const llmSpans = nodeGroupSpans.filter(s => s.kind === 'llm' && s.llm)
            const totalTokens = llmSpans.reduce((sum, s) => sum + (s.llm?.tokens.total_tokens ?? 0), 0)
            const allTokens = spans
              .filter(s => s.kind === 'llm' && s.llm)
              .reduce((sum, s) => sum + (s.llm?.tokens.total_tokens ?? 0), 0)

            if (totalTokens === 0 || allTokens === 0) return null

            const percentage = (totalTokens / allTokens) * 100

            return (
              <div
                key={nodeId}
                className="bg-purple-500 border-r border-white last:border-r-0"
                style={{ width: `${percentage}%` }}
                title={`${nodeId}: ${formatTokens(totalTokens)} tokens (${percentage.toFixed(1)}%)`}
              />
            )
          })}
        </div>
      </div>
    </div>
  )
}
