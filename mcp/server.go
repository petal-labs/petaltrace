package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/petal-labs/petaltrace/diff"
	"github.com/petal-labs/petaltrace/pricing"
	"github.com/petal-labs/petaltrace/replay"
	"github.com/petal-labs/petaltrace/store"
)

// Server implements an MCP server for PetalTrace
type Server struct {
	store        store.TraceStore
	pricingTable *pricing.PricingTable
	diffEngine   *diff.Engine
	replayEngine *replay.Engine
	logger       *slog.Logger

	tools map[string]*Tool

	stdin  io.Reader
	stdout io.Writer

	mu     sync.Mutex
	closed bool
}

// ServerConfig configures the MCP server
type ServerConfig struct {
	Store        store.TraceStore
	PricingTable *pricing.PricingTable
	DiffEngine   *diff.Engine
	ReplayEngine *replay.Engine
	Logger       *slog.Logger
	Stdin        io.Reader
	Stdout       io.Writer
}

// NewServer creates a new MCP server
func NewServer(cfg ServerConfig) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Stdin == nil {
		cfg.Stdin = os.Stdin
	}
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}

	s := &Server{
		store:        cfg.Store,
		pricingTable: cfg.PricingTable,
		diffEngine:   cfg.DiffEngine,
		replayEngine: cfg.ReplayEngine,
		logger:       cfg.Logger,
		stdin:        cfg.Stdin,
		stdout:       cfg.Stdout,
		tools:        make(map[string]*Tool),
	}

	// Register all tools
	s.registerTools()

	return s
}

// Tool represents an MCP tool
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Handler     ToolHandler     `json:"-"`
}

// ToolHandler processes a tool invocation
type ToolHandler func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

// registerTools registers all PetalTrace MCP tools
func (s *Server) registerTools() {
	// Trace tools
	s.registerTool(s.traceListTool())
	s.registerTool(s.traceGetTool())
	s.registerTool(s.traceSearchTool())

	// Prompt tools
	s.registerTool(s.promptGetTool())

	// Cost tools
	s.registerTool(s.costSummaryTool())
	s.registerTool(s.costRunTool())

	// Diff tools
	s.registerTool(s.diffCompareTool())

	// Replay tools
	s.registerTool(s.runReplayTool())
}

// registerTool adds a tool to the server
func (s *Server) registerTool(tool *Tool) {
	s.tools[tool.Name] = tool
}

// Run starts the MCP server and processes requests
func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("starting MCP server", "transport", "stdio")

	scanner := bufio.NewScanner(s.stdin)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB max message

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "Parse error", err.Error())
			continue
		}

		s.handleRequest(ctx, &req)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}

	return nil
}

// Close closes the MCP server
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	s.logger.Info("closing MCP server")
	return nil
}

// Request represents a JSON-RPC request
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC response
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorObject    `json:"error,omitempty"`
}

// ErrorObject represents a JSON-RPC error
type ErrorObject struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// handleRequest processes an incoming MCP request
func (s *Server) handleRequest(ctx context.Context, req *Request) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(ctx, req)
	case "ping":
		s.handlePing(req)
	case "notifications/cancelled":
		// Ignore cancellation notifications
	default:
		s.sendError(req.ID, -32601, "Method not found", req.Method)
	}
}

// InitializeParams contains initialization parameters
type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	ClientInfo      ClientInfo     `json:"clientInfo,omitempty"`
}

// ClientInfo contains client information
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// InitializeResult contains initialization result
type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo     `json:"serverInfo"`
}

// ServerCapabilities describes server capabilities
type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ToolsCapability describes tool capabilities
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ServerInfo contains server information
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// handleInitialize handles the initialize request
func (s *Server) handleInitialize(req *Request) {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{
				ListChanged: false,
			},
		},
		ServerInfo: ServerInfo{
			Name:    "petaltrace",
			Version: "0.1.0",
		},
	}

	s.sendResult(req.ID, result)
}

// ToolsListResult contains the tools list
type ToolsListResult struct {
	Tools []ToolInfo `json:"tools"`
}

// ToolInfo describes a tool
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// handleToolsList handles the tools/list request
func (s *Server) handleToolsList(req *Request) {
	tools := make([]ToolInfo, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}

	s.sendResult(req.ID, ToolsListResult{Tools: tools})
}

// ToolsCallParams contains tool call parameters
type ToolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolsCallResult contains tool call result
type ToolsCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock represents a content block in the result
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// handleToolsCall handles the tools/call request
func (s *Server) handleToolsCall(ctx context.Context, req *Request) {
	var params ToolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "Invalid params", err.Error())
		return
	}

	tool, ok := s.tools[params.Name]
	if !ok {
		s.sendError(req.ID, -32602, "Unknown tool", params.Name)
		return
	}

	result, err := tool.Handler(ctx, params.Arguments)
	if err != nil {
		s.sendResult(req.ID, ToolsCallResult{
			Content: []ContentBlock{{Type: "text", Text: err.Error()}},
			IsError: true,
		})
		return
	}

	// Format result as JSON text
	resultText := string(result)
	s.sendResult(req.ID, ToolsCallResult{
		Content: []ContentBlock{{Type: "text", Text: resultText}},
	})
}

// handlePing handles the ping request
func (s *Server) handlePing(req *Request) {
	s.sendResult(req.ID, map[string]string{})
}

// sendResult sends a successful response
func (s *Server) sendResult(id *json.RawMessage, result any) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		s.logger.Error("failed to marshal result", "error", err)
		return
	}

	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  resultJSON,
	}

	s.sendResponse(&resp)
}

// sendError sends an error response
func (s *Server) sendError(id *json.RawMessage, code int, message, data string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &ErrorObject{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	s.sendResponse(&resp)
}

// sendResponse sends a response to stdout
func (s *Server) sendResponse(resp *Response) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error("failed to marshal response", "error", err)
		return
	}

	fmt.Fprintf(s.stdout, "%s\n", data)
}
