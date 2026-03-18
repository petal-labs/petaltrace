import { useMemo } from 'react'
import ReactFlow, {
  Node,
  Edge,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  MarkerType,
  Position,
} from 'reactflow'
import 'reactflow/dist/style.css'
import { formatDuration, formatTokens, formatCost, cn } from '@/lib/utils'
import type { Span, SpanKind } from '@/types'

interface GraphViewProps {
  spans: Span[]
  onSelectSpan: (span: Span | null) => void
  selectedSpanId?: string
}

const kindColors: Record<SpanKind, { bg: string; border: string; text: string }> = {
  node: { bg: 'bg-blue-50', border: 'border-blue-300', text: 'text-blue-800' },
  llm: { bg: 'bg-purple-50', border: 'border-purple-300', text: 'text-purple-800' },
  tool: { bg: 'bg-orange-50', border: 'border-orange-300', text: 'text-orange-800' },
  edge: { bg: 'bg-gray-50', border: 'border-gray-300', text: 'text-gray-800' },
  custom: { bg: 'bg-gray-50', border: 'border-gray-300', text: 'text-gray-800' },
}


export default function GraphView({ spans, onSelectSpan, selectedSpanId }: GraphViewProps) {
  // Build nodes from spans
  const { nodes: initialNodes, edges: initialEdges } = useMemo(() => {
    const nodeSpans = spans.filter((s) => s.kind === 'node')
    const edgeSpans = spans.filter((s) => s.kind === 'edge')
    const llmSpans = spans.filter((s) => s.kind === 'llm')
    const toolSpans = spans.filter((s) => s.kind === 'tool')

    // Create a map of node ID to span for quick lookup
    const nodeMap = new Map<string, Span>()
    nodeSpans.forEach((span) => {
      const nodeId = span.node?.node_id ?? span.name
      nodeMap.set(nodeId, span)
    })

    // Calculate LLM and tool stats per node
    const nodeStats = new Map<string, { tokens: number; cost: number; llmCount: number; toolCount: number }>()

    for (const span of llmSpans) {
      const parent = spans.find((s) => s.id === span.parent_id)
      if (parent?.kind === 'node') {
        const nodeId = parent.node?.node_id ?? parent.name
        const existing = nodeStats.get(nodeId) ?? { tokens: 0, cost: 0, llmCount: 0, toolCount: 0 }
        existing.tokens += span.llm?.tokens.total_tokens ?? 0
        existing.cost += span.llm?.tokens.cost_estimate ?? 0
        existing.llmCount += 1
        nodeStats.set(nodeId, existing)
      }
    }

    for (const span of toolSpans) {
      const parent = spans.find((s) => s.id === span.parent_id)
      if (parent?.kind === 'node') {
        const nodeId = parent.node?.node_id ?? parent.name
        const existing = nodeStats.get(nodeId) ?? { tokens: 0, cost: 0, llmCount: 0, toolCount: 0 }
        existing.toolCount += 1
        nodeStats.set(nodeId, existing)
      }
    }

    // Layout nodes in a grid
    const nodeIds = Array.from(nodeMap.keys())
    const cols = Math.ceil(Math.sqrt(nodeIds.length))
    const nodeWidth = 220
    const nodeHeight = 120
    const gap = 80

    const nodes: Node[] = nodeIds.map((nodeId, index) => {
      const span = nodeMap.get(nodeId)!
      const stats = nodeStats.get(nodeId)
      const row = Math.floor(index / cols)
      const col = index % cols

      return {
        id: nodeId,
        type: 'custom',
        position: { x: col * (nodeWidth + gap), y: row * (nodeHeight + gap) },
        data: {
          span,
          stats,
          selected: span.id === selectedSpanId,
          onClick: () => onSelectSpan(span),
        },
        sourcePosition: Position.Right,
        targetPosition: Position.Left,
      }
    })

    // Create edges from edge spans
    const edges: Edge[] = edgeSpans
      .filter((span) => span.edge?.source_node && span.edge?.target_node)
      .map((span) => ({
        id: span.id,
        source: span.edge!.source_node,
        target: span.edge!.target_node,
        type: 'smoothstep',
        animated: false,
        markerEnd: {
          type: MarkerType.ArrowClosed,
          color: '#9ca3af',
        },
        style: {
          stroke: '#9ca3af',
          strokeWidth: 2,
        },
        label: span.edge?.data_preview ? `${formatBytes(span.edge.data_size_bytes)}` : undefined,
        labelStyle: { fontSize: 10, fill: '#6b7280' },
      }))

    return { nodes, edges }
  }, [spans, selectedSpanId, onSelectSpan])

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)

  // Update nodes when selection changes
  useMemo(() => {
    setNodes(initialNodes)
    setEdges(initialEdges)
  }, [initialNodes, initialEdges, setNodes, setEdges])

  const nodeTypes = useMemo(() => ({ custom: CustomNode }), [])

  if (spans.length === 0) {
    return (
      <div className="card p-8 text-center text-gray-500">
        No graph data available
      </div>
    )
  }

  return (
    <div className="card overflow-hidden" style={{ height: 600 }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        attributionPosition="bottom-left"
      >
        <Background color="#e5e7eb" gap={16} />
        <Controls />
        <MiniMap
          nodeColor={(node) => {
            const span = node.data.span as Span
            return span.status === 'error' ? '#ef4444' : '#3b82f6'
          }}
          maskColor="rgba(0, 0, 0, 0.1)"
        />
      </ReactFlow>
    </div>
  )
}

// Custom node component
function CustomNode({ data }: { data: { span: Span; stats?: { tokens: number; cost: number; llmCount: number; toolCount: number }; selected: boolean; onClick: () => void } }) {
  const { span, stats, selected, onClick } = data
  const nodeId = span.node?.node_id ?? span.name
  const nodeType = span.node?.node_type ?? 'node'
  const colors = kindColors.node

  return (
    <div
      onClick={onClick}
      className={cn(
        'px-4 py-3 rounded-lg border-2 shadow-sm cursor-pointer transition-all min-w-[200px]',
        colors.bg,
        colors.border,
        span.status === 'error' && 'border-red-500',
        selected && 'ring-2 ring-primary-400 ring-offset-2'
      )}
    >
      {/* Header */}
      <div className="flex items-center justify-between mb-2">
        <div className="font-medium text-gray-900 truncate">{nodeId}</div>
        <span className={cn(
          'w-2 h-2 rounded-full',
          span.status === 'ok' ? 'bg-green-500' : 'bg-red-500'
        )} />
      </div>

      {/* Type */}
      <div className="text-xs text-gray-500 mb-2">{nodeType}</div>

      {/* Stats */}
      <div className="flex items-center gap-3 text-xs text-gray-600">
        <span title="Duration">{formatDuration(span.duration_ms)}</span>

        {stats && stats.tokens > 0 && (
          <span title="Tokens" className="flex items-center gap-1">
            <span className="w-2 h-2 rounded-full bg-purple-500" />
            {formatTokens(stats.tokens)}
          </span>
        )}

        {stats && stats.llmCount > 0 && (
          <span title="LLM calls" className="text-purple-600">
            {stats.llmCount} LLM
          </span>
        )}

        {stats && stats.toolCount > 0 && (
          <span title="Tool calls" className="text-orange-600">
            {stats.toolCount} tool
          </span>
        )}
      </div>

      {/* Cost */}
      {stats && stats.cost > 0 && (
        <div className="text-xs text-gray-500 mt-1">
          {formatCost(stats.cost)}
        </div>
      )}
    </div>
  )
}

function formatBytes(bytes: number): string {
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${bytes} B`
}
