package store

import (
	"encoding/json"
	"time"
)

type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

type Run struct {
	ID              string          `json:"id"`
	WorkflowID      string          `json:"workflow_id"`
	WorkflowName    string          `json:"workflow_name"`
	WorkflowVersion string          `json:"workflow_version,omitempty"`
	SourceKind      string          `json:"source_kind"`
	Status          RunStatus       `json:"status"`
	StartedAt       time.Time       `json:"started_at"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
	DurationMs      int64           `json:"duration_ms"`

	GraphSnapshot  json.RawMessage `json:"graph_snapshot,omitempty"`
	InputSnapshot  json.RawMessage `json:"input_snapshot,omitempty"`
	ConfigSnapshot json.RawMessage `json:"config_snapshot,omitempty"`

	TotalTokens   TokenSummary `json:"total_tokens"`
	EstimatedCost CostEstimate `json:"estimated_cost"`
	NodeCount     int          `json:"node_count"`
	ErrorCount    int          `json:"error_count"`

	Tags          map[string]string `json:"tags,omitempty"`
	TriggerSource string            `json:"trigger_source,omitempty"`
	ParentRunID   *string           `json:"parent_run_id,omitempty"`
	Starred       bool              `json:"starred"`

	CreatedAt time.Time `json:"created_at"`
}

type SpanKind string

const (
	SpanKindNode   SpanKind = "node"
	SpanKindLLM    SpanKind = "llm"
	SpanKindTool   SpanKind = "tool"
	SpanKindEdge   SpanKind = "edge"
	SpanKindCustom SpanKind = "custom"
)

type SpanStatus string

const (
	SpanStatusOK    SpanStatus = "ok"
	SpanStatusError SpanStatus = "error"
)

type Span struct {
	ID          string     `json:"id"`
	RunID       string     `json:"run_id"`
	ParentID    *string    `json:"parent_id,omitempty"`
	TraceID     string     `json:"trace_id"`
	Kind        SpanKind   `json:"kind"`
	Name        string     `json:"name"`
	Status      SpanStatus `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	DurationMs  int64      `json:"duration_ms"`

	Node *NodeSpanData `json:"node,omitempty"`
	LLM  *LLMSpanData  `json:"llm,omitempty"`
	Tool *ToolSpanData `json:"tool,omitempty"`
	Edge *EdgeSpanData `json:"edge,omitempty"`

	Error *SpanError `json:"error,omitempty"`

	Attributes map[string]any `json:"attributes,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

type NodeSpanData struct {
	NodeID     string          `json:"node_id"`
	NodeType   string          `json:"node_type"`
	Inputs     json.RawMessage `json:"inputs,omitempty"`
	Outputs    json.RawMessage `json:"outputs,omitempty"`
	Config     json.RawMessage `json:"config,omitempty"`
	RetryCount int             `json:"retry_count"`
}

type LLMSpanData struct {
	Provider        string           `json:"provider"`
	Model           string           `json:"model"`
	SystemPrompt    string           `json:"system_prompt,omitempty"`
	Messages        []LLMMessage     `json:"messages,omitempty"`
	Temperature     *float64         `json:"temperature,omitempty"`
	MaxTokens       *int             `json:"max_tokens,omitempty"`
	ToolDefinitions []ToolDefinition `json:"tool_definitions,omitempty"`
	Completion      LLMCompletion    `json:"completion"`
	Tokens          TokenDetail      `json:"tokens"`
	TimeToFirstToken *int64          `json:"ttft_ms,omitempty"`
	TotalLatency    int64            `json:"total_latency_ms"`
	RequestID       string           `json:"request_id,omitempty"`
	StopReason      string           `json:"stop_reason,omitempty"`
	CacheRead       *int             `json:"cache_read_tokens,omitempty"`
	CacheCreation   *int             `json:"cache_creation_tokens,omitempty"`
}

type LLMMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type LLMCompletion struct {
	Content     json.RawMessage `json:"content,omitempty"`
	TextContent string          `json:"text_content,omitempty"`
}

type TokenDetail struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostEstimate float64 `json:"cost_estimate"`
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type ToolSpanData struct {
	ToolName   string          `json:"tool_name"`
	ActionName string          `json:"action_name"`
	Origin     string          `json:"origin"`
	Inputs     json.RawMessage `json:"inputs,omitempty"`
	Outputs    json.RawMessage `json:"outputs,omitempty"`
	DurationMs int64           `json:"duration_ms"`
	InvokedBy  *string         `json:"invoked_by,omitempty"`
	ToolUseID  *string         `json:"tool_use_id,omitempty"`
}

type EdgeSpanData struct {
	SourceNode  string `json:"source_node"`
	SourcePort  string `json:"source_port"`
	TargetNode  string `json:"target_node"`
	TargetPort  string `json:"target_port"`
	DataSize    int64  `json:"data_size_bytes"`
	DataPreview string `json:"data_preview,omitempty"`
}

type SpanError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

type TokenSummary struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type CostEstimate struct {
	Currency   string             `json:"currency"`
	Total      float64            `json:"total"`
	ByProvider map[string]float64 `json:"by_provider,omitempty"`
	ByModel    map[string]float64 `json:"by_model,omitempty"`
	ByNode     map[string]float64 `json:"by_node,omitempty"`
}

type RunDiff struct {
	ID           string      `json:"id"`
	BaseRunID    string      `json:"base_run_id"`
	CompareRunID string      `json:"compare_run_id"`
	Summary      DiffSummary `json:"summary"`
	NodeDiffs    []NodeDiff  `json:"node_diffs"`
	GraphDiff    *GraphDiff  `json:"graph_diff,omitempty"`
	CostDiff     CostDiff    `json:"cost_diff"`
	CreatedAt    time.Time   `json:"created_at"`
}

type DiffSummary struct {
	StatusMatch    bool    `json:"status_match"`
	DurationDelta  int64   `json:"duration_delta_ms"`
	TokenDelta     int     `json:"token_delta"`
	CostDelta      float64 `json:"cost_delta"`
	NodeDiffCount  int     `json:"node_diff_count"`
	PathDivergence bool    `json:"path_divergence"`
}

type NodeDiff struct {
	NodeID          string     `json:"node_id"`
	NodeType        string     `json:"node_type"`
	Status          string     `json:"status"`
	PromptDiff      *TextDiff  `json:"prompt_diff,omitempty"`
	OutputDiff      *TextDiff  `json:"output_diff,omitempty"`
	TokenDiff       *TokenDiff `json:"token_diff,omitempty"`
	InputDiff       *DataDiff  `json:"input_diff,omitempty"`
	ResultDiff      *DataDiff  `json:"result_diff,omitempty"`
	DurationBase    int64      `json:"duration_base_ms"`
	DurationCompare int64      `json:"duration_compare_ms"`
}

type TextDiff struct {
	BaseText    string  `json:"base_text"`
	CompareText string  `json:"compare_text"`
	Hunks       []Hunk  `json:"hunks,omitempty"`
	Similarity  float64 `json:"similarity"`
}

type Hunk struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
}

type TokenDiff struct {
	BaseTokens    TokenDetail `json:"base_tokens"`
	CompareTokens TokenDetail `json:"compare_tokens"`
	Delta         int         `json:"delta"`
}

type DataDiff struct {
	BaseData    json.RawMessage `json:"base_data,omitempty"`
	CompareData json.RawMessage `json:"compare_data,omitempty"`
	Changed     bool            `json:"changed"`
}

type GraphDiff struct {
	NodesAdded   []string `json:"nodes_added,omitempty"`
	NodesRemoved []string `json:"nodes_removed,omitempty"`
	EdgesChanged []string `json:"edges_changed,omitempty"`
}

type CostDiff struct {
	BaseCost      float64            `json:"base_cost"`
	CompareCost   float64            `json:"compare_cost"`
	Delta         float64            `json:"delta"`
	ByProvider    map[string]float64 `json:"by_provider,omitempty"`
	ByModel       map[string]float64 `json:"by_model,omitempty"`
}

type ListRunsOptions struct {
	WorkflowID    string
	WorkflowName  string
	Status        RunStatus
	Since         *time.Time
	Until         *time.Time
	MinCost       *float64
	Tags          map[string]string
	SourceKind    string
	TriggerSource string
	Starred       *bool
	Cursor        string
	Limit         int
	SortBy        string
	SortOrder     string
}

type PricingEntry struct {
	Provider          string  `json:"provider"`
	Model             string  `json:"model"`
	InputPer1M        float64 `json:"input_per_1m"`
	OutputPer1M       float64 `json:"output_per_1m"`
	CacheReadPer1M    float64 `json:"cache_read_per_1m,omitempty"`
	CacheWritePer1M   float64 `json:"cache_write_per_1m,omitempty"`
	EffectiveFrom     time.Time `json:"effective_from"`
}
