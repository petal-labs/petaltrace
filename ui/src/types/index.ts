export type RunStatus = 'running' | 'completed' | 'failed' | 'cancelled'
export type SpanKind = 'node' | 'llm' | 'tool' | 'edge' | 'custom'
export type SpanStatus = 'ok' | 'error'

export interface TokenSummary {
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  total_tokens: number
}

export interface CostEstimate {
  currency: string
  total: number
  by_provider?: Record<string, number>
  by_model?: Record<string, number>
  by_node?: Record<string, number>
}

export interface Run {
  id: string
  workflow_id: string
  workflow_name: string
  workflow_version?: string
  source_kind: string
  status: RunStatus
  started_at: string
  completed_at?: string
  duration_ms: number
  total_tokens: TokenSummary
  estimated_cost: CostEstimate
  node_count: number
  error_count: number
  tags?: Record<string, string>
  trigger_source?: string
  parent_run_id?: string
}

export interface SpanError {
  code?: string
  message: string
  details?: string
}

export interface NodeSpanData {
  node_id: string
  node_type: string
  inputs?: unknown
  outputs?: unknown
  config?: unknown
  retry_count: number
}

export interface LLMSpanData {
  provider: string
  model: string
  system_prompt?: string
  messages?: LLMMessage[]
  temperature?: number
  max_tokens?: number
  completion: LLMCompletion
  tokens: TokenDetail
  ttft_ms?: number
  total_latency_ms: number
  stop_reason?: string
  cache_read_tokens?: number
  cache_creation_tokens?: number
}

export interface LLMMessage {
  role: string
  content: unknown
}

export interface LLMCompletion {
  content?: unknown
  text_content?: string
}

export interface TokenDetail {
  input_tokens: number
  output_tokens: number
  total_tokens: number
  cost_estimate: number
}

export interface ToolSpanData {
  tool_name: string
  action_name: string
  origin: string
  inputs?: unknown
  outputs?: unknown
  duration_ms: number
  invoked_by?: string
  tool_use_id?: string
}

export interface EdgeSpanData {
  source_node: string
  source_port: string
  target_node: string
  target_port: string
  data_size_bytes: number
  data_preview?: string
}

export interface Span {
  id: string
  run_id: string
  parent_id?: string
  trace_id: string
  kind: SpanKind
  name: string
  status: SpanStatus
  started_at: string
  completed_at?: string
  duration_ms: number
  node?: NodeSpanData
  llm?: LLMSpanData
  tool?: ToolSpanData
  edge?: EdgeSpanData
  error?: SpanError
  attributes?: Record<string, unknown>
}

export interface RunDiff {
  id: string
  base_run_id: string
  compare_run_id: string
  summary: DiffSummary
  node_diffs: NodeDiff[]
  graph_diff?: GraphDiff
  cost_diff: CostDiff
}

export interface DiffSummary {
  status_match: boolean
  duration_delta_ms: number
  token_delta: number
  cost_delta: number
  node_diff_count: number
  path_divergence: boolean
}

export interface NodeDiff {
  node_id: string
  node_type: string
  status: string
  output_diff?: TextDiff
  token_diff?: {
    base_tokens: TokenDetail
    compare_tokens: TokenDetail
    delta: number
  }
  duration_base_ms: number
  duration_compare_ms: number
}

export interface TextDiff {
  base_text: string
  compare_text: string
  hunks?: Hunk[]
  similarity: number
}

export interface Hunk {
  start_line: number
  end_line: number
  content: string
}

export interface GraphDiff {
  nodes_added?: string[]
  nodes_removed?: string[]
  edges_changed?: string[]
}

export interface CostDiff {
  base_cost: number
  compare_cost: number
  delta: number
  by_provider?: Record<string, number>
  by_model?: Record<string, number>
}

export interface PromptData {
  run_id: string
  node_id: string
  span_id: string
  provider: string
  model: string
  system_prompt?: string
  messages: LLMMessage[]
  completion?: LLMCompletion
  tokens: TokenDetail
  latency_ms: number
  stop_reason?: string
}

export interface CostSummary {
  total_runs: number
  total_tokens: number
  total_cost: number
  input_tokens: number
  output_tokens: number
  by_workflow: CostBreakdown[]
  by_provider: CostBreakdown[]
  by_model: CostBreakdown[]
}

export interface CostBreakdown {
  name: string
  run_count: number
  total_tokens: number
  total_cost: number
}

export interface Replay {
  replay_id: string
  source_run_id: string
  new_run_id?: string
  diff_id?: string
  mode: 'live' | 'mocked' | 'hybrid'
  status: 'pending' | 'running' | 'completed' | 'failed'
  error?: string
  started_at: number
  completed_at?: number
}
