import type { Run, Span, RunDiff, PromptData, CostSummary, Replay } from '@/types'

const API_BASE = '/api'

async function fetchJSON<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
  })

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Request failed')
  }

  return response.json()
}

// Runs
export interface ListRunsParams {
  workflow?: string
  status?: string
  since?: string
  until?: string
  limit?: number
  cursor?: string
}

export interface ListRunsResponse {
  data: Run[]
  cursor?: string
  has_more: boolean
  total_count?: number
}

export async function listRuns(params: ListRunsParams = {}): Promise<ListRunsResponse> {
  const searchParams = new URLSearchParams()
  if (params.workflow) searchParams.set('workflow', params.workflow)
  if (params.status) searchParams.set('status', params.status)
  if (params.since) searchParams.set('since', params.since)
  if (params.until) searchParams.set('until', params.until)
  if (params.limit) searchParams.set('limit', String(params.limit))
  if (params.cursor) searchParams.set('cursor', params.cursor)

  const query = searchParams.toString()
  return fetchJSON<ListRunsResponse>(`/runs${query ? `?${query}` : ''}`)
}

export async function getRun(id: string): Promise<Run> {
  return fetchJSON<Run>(`/runs/${id}`)
}

export async function deleteRun(id: string): Promise<void> {
  await fetchJSON(`/runs/${id}`, { method: 'DELETE' })
}

// Spans
export async function getSpans(runId: string): Promise<Span[]> {
  const response = await fetchJSON<{ spans: Span[] }>(`/runs/${runId}/spans`)
  return response.spans
}

export async function getSpan(runId: string, spanId: string): Promise<Span> {
  return fetchJSON<Span>(`/runs/${runId}/spans/${spanId}`)
}

// Prompts
export async function getPrompt(runId: string, nodeId: string): Promise<PromptData> {
  return fetchJSON<PromptData>(`/runs/${runId}/prompts/${nodeId}`)
}

// Diff
export interface ComputeDiffParams {
  base_run_id: string
  compare_run_id: string
  include_content?: boolean
  include_similarity?: boolean
}

export async function computeDiff(params: ComputeDiffParams): Promise<RunDiff> {
  return fetchJSON<RunDiff>('/diff', {
    method: 'POST',
    body: JSON.stringify(params),
  })
}

export async function getDiff(id: string): Promise<RunDiff> {
  return fetchJSON<RunDiff>(`/diff/${id}`)
}

export async function getDiffByRuns(baseRunId: string, compareRunId: string): Promise<RunDiff | null> {
  try {
    return await fetchJSON<RunDiff>(`/diff/runs?base_run_id=${baseRunId}&compare_run_id=${compareRunId}`)
  } catch {
    return null
  }
}

// Cost
export interface CostSummaryParams {
  workflow?: string
  since?: string
  until?: string
  group_by?: 'workflow' | 'provider' | 'model'
}

export async function getCostSummary(params: CostSummaryParams = {}): Promise<CostSummary> {
  const searchParams = new URLSearchParams()
  if (params.workflow) searchParams.set('workflow', params.workflow)
  if (params.since) searchParams.set('since', params.since)
  if (params.until) searchParams.set('until', params.until)
  if (params.group_by) searchParams.set('group_by', params.group_by)

  const query = searchParams.toString()
  return fetchJSON<CostSummary>(`/cost/summary${query ? `?${query}` : ''}`)
}

export async function getRunCost(runId: string): Promise<CostSummary> {
  return fetchJSON<CostSummary>(`/cost/runs/${runId}`)
}

export interface CostTimeseriesParams {
  workflow?: string
  since?: string
  until?: string
  bucket?: 'hour' | 'day' | 'week'
}

export interface CostTimeseriesPoint {
  timestamp: string
  cost: number
  tokens: number
  runs: number
}

export async function getCostTimeseries(params: CostTimeseriesParams = {}): Promise<CostTimeseriesPoint[]> {
  const searchParams = new URLSearchParams()
  if (params.workflow) searchParams.set('workflow', params.workflow)
  if (params.since) searchParams.set('since', params.since)
  if (params.until) searchParams.set('until', params.until)
  if (params.bucket) searchParams.set('bucket', params.bucket)

  const query = searchParams.toString()
  const response = await fetchJSON<{ data: CostTimeseriesPoint[] }>(`/cost/timeseries${query ? `?${query}` : ''}`)
  return response.data
}

// Replay
export interface TriggerReplayParams {
  source_run_id: string
  mode?: 'live' | 'mocked' | 'hybrid'
  model?: string
  temperature?: number
  max_tokens?: number
  tags?: Record<string, string>
  auto_diff?: boolean
  sync?: boolean
}

export async function triggerReplay(params: TriggerReplayParams): Promise<Replay> {
  return fetchJSON<Replay>('/replay', {
    method: 'POST',
    body: JSON.stringify(params),
  })
}

export async function getReplayStatus(id: string): Promise<Replay> {
  return fetchJSON<Replay>(`/replay/${id}`)
}

export async function listReplays(sourceRunId?: string): Promise<Replay[]> {
  const query = sourceRunId ? `?source_run_id=${sourceRunId}` : ''
  const response = await fetchJSON<{ replays: Replay[] }>(`/replays${query}`)
  return response.replays
}

// SSE
export function subscribeToRun(runId: string, onEvent: (event: MessageEvent) => void): EventSource {
  const source = new EventSource(`${API_BASE}/runs/${runId}/stream`)
  source.onmessage = onEvent
  return source
}

export function subscribeToLive(onEvent: (event: MessageEvent) => void): EventSource {
  const source = new EventSource(`${API_BASE}/live`)
  source.onmessage = onEvent
  return source
}

// Health
export interface HealthResponse {
  status: string
  timestamp: string
  version: string
}

export async function getHealth(): Promise<HealthResponse> {
  return fetchJSON<HealthResponse>('/health')
}

// Stats
export interface StatsResponse {
  database_size_bytes: number
  run_count: number
  span_count: number
  diff_count: number
  oldest_run?: string
  newest_run?: string
  top_workflows: { workflow_name: string; run_count: number }[]
}

export async function getStats(): Promise<StatsResponse> {
  return fetchJSON<StatsResponse>('/stats')
}
