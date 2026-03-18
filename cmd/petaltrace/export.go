package petaltrace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"

	"github.com/petal-labs/petaltrace/store"
)

var exportCmd = &cobra.Command{
	Use:   "export <run-id> [output-file]",
	Short: "Export a run to a JSON file",
	Long: `Export a run and all its spans to a JSON file for sharing or backup.

Examples:
  petaltrace export run-abc123
  petaltrace export run-abc123 run-abc123.json
  petaltrace export run-abc123 --include-search-text`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runExport,
}

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import a run from a JSON file",
	Long: `Import a previously exported run and its spans.

Examples:
  petaltrace import run-abc123.json
  petaltrace import run-abc123.json --new-id`,
	Args: cobra.ExactArgs(1),
	RunE: runImport,
}

var (
	exportIncludeSearchText bool
	importNewID             bool
)

func init() {
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)

	exportCmd.Flags().BoolVar(&exportIncludeSearchText, "include-search-text", false, "Include extracted search text")
	importCmd.Flags().BoolVar(&importNewID, "new-id", false, "Generate new run and span IDs on import")
}

// ExportedRun represents a run with all its data for export/import
type ExportedRun struct {
	Version     string            `json:"version"`
	ExportedAt  time.Time         `json:"exported_at"`
	Run         *store.Run        `json:"run"`
	Spans       []*store.Span     `json:"spans"`
	SearchTexts []ExportedFTSText `json:"search_texts,omitempty"`
}

type ExportedFTSText struct {
	SpanID     string `json:"span_id"`
	PromptText string `json:"prompt_text"`
	Completion string `json:"completion"`
}

func runExport(cmd *cobra.Command, args []string) error {
	runID := args[0]

	// Determine output file
	outputFile := fmt.Sprintf("%s.json", runID)
	if len(args) > 1 {
		outputFile = args[1]
	}

	traceStore, err := openStore()
	if err != nil {
		return err
	}
	defer traceStore.Close()

	ctx := context.Background()

	// Get run
	run, err := traceStore.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("getting run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("run not found: %s", runID)
	}

	// Get all spans
	spans, err := traceStore.GetSpanTree(ctx, runID)
	if err != nil {
		return fmt.Errorf("getting spans: %w", err)
	}

	// Build export object
	export := &ExportedRun{
		Version:    "1.0",
		ExportedAt: time.Now(),
		Run:        run,
		Spans:      spans,
	}

	// Extract search text from LLM spans if requested
	if exportIncludeSearchText {
		for _, span := range spans {
			if span.LLM != nil {
				promptText := extractSearchablePromptText(span.LLM)
				completion := extractSearchableCompletionText(span.LLM)

				if promptText != "" || completion != "" {
					export.SearchTexts = append(export.SearchTexts, ExportedFTSText{
						SpanID:     span.ID,
						PromptText: promptText,
						Completion: completion,
					})
				}
			}
		}
	}

	// Write to file
	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(export); err != nil {
		return fmt.Errorf("encoding export: %w", err)
	}

	fmt.Printf("Exported run %s to %s\n", runID, outputFile)
	fmt.Printf("  Spans: %d\n", len(spans))

	return nil
}

func extractSearchablePromptText(llm *store.LLMSpanData) string {
	var text string

	if llm.SystemPrompt != "" {
		text = llm.SystemPrompt + "\n"
	}

	for _, msg := range llm.Messages {
		extracted := extractTextFromRawMessage(msg.Content)
		if extracted != "" {
			text += extracted + "\n"
		}
	}

	return text
}

func extractSearchableCompletionText(llm *store.LLMSpanData) string {
	// Use TextContent if available
	if llm.Completion.TextContent != "" {
		return llm.Completion.TextContent
	}

	// Otherwise try to extract from Content
	return extractTextFromRawMessage(llm.Completion.Content)
}

func runImport(cmd *cobra.Command, args []string) error {
	inputFile := args[0]

	// Read file
	f, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	var export ExportedRun
	if err := json.NewDecoder(f).Decode(&export); err != nil {
		return fmt.Errorf("decoding import: %w", err)
	}

	if export.Run == nil {
		return fmt.Errorf("invalid export file: missing run data")
	}

	traceStore, err := openStore()
	if err != nil {
		return err
	}
	defer traceStore.Close()

	ctx := context.Background()

	// Generate new IDs if requested
	idMapping := make(map[string]string) // old ID -> new ID
	if importNewID {
		// Generate new run ID
		newRunID := generateID()
		idMapping[export.Run.ID] = newRunID
		export.Run.ID = newRunID

		// Generate new span IDs
		for _, span := range export.Spans {
			newSpanID := generateID()
			idMapping[span.ID] = newSpanID
			span.ID = newSpanID
			span.RunID = newRunID

			// Update parent ID if present
			if span.ParentID != nil {
				if newParentID, ok := idMapping[*span.ParentID]; ok {
					span.ParentID = &newParentID
				}
			}
		}

		// Update search texts
		for i := range export.SearchTexts {
			if newSpanID, ok := idMapping[export.SearchTexts[i].SpanID]; ok {
				export.SearchTexts[i].SpanID = newSpanID
			}
		}
	}

	// Check if run already exists
	existing, err := traceStore.GetRun(ctx, export.Run.ID)
	if err != nil {
		return fmt.Errorf("checking existing run: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("run %s already exists (use --new-id to import with new ID)", export.Run.ID)
	}

	// Import run
	if err := traceStore.CreateRun(ctx, export.Run); err != nil {
		return fmt.Errorf("creating run: %w", err)
	}

	// Import spans in batches
	if len(export.Spans) > 0 {
		if err := traceStore.CreateSpanBatch(ctx, export.Spans); err != nil {
			return fmt.Errorf("creating spans: %w", err)
		}
	}

	// Import search texts
	for _, st := range export.SearchTexts {
		if err := traceStore.IndexSpanText(ctx, st.SpanID, st.PromptText, st.Completion); err != nil {
			// Log but don't fail on FTS indexing errors
			fmt.Fprintf(os.Stderr, "Warning: failed to index text for span %s: %v\n", st.SpanID, err)
		}
	}

	fmt.Printf("Imported run %s from %s\n", export.Run.ID, inputFile)
	fmt.Printf("  Spans: %d\n", len(export.Spans))
	if importNewID {
		fmt.Printf("  (New IDs generated)\n")
	}

	return nil
}

func generateID() string {
	return ulid.Make().String()
}
