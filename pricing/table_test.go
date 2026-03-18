package pricing

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBuiltin(t *testing.T) {
	table, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin() error = %v", err)
	}

	providers := table.Providers()
	if len(providers) == 0 {
		t.Error("no providers loaded")
	}

	hasAnthropic := false
	for _, p := range providers {
		if p == "anthropic" {
			hasAnthropic = true
			break
		}
	}
	if !hasAnthropic {
		t.Error("anthropic provider not found")
	}
}

func TestGetPricing(t *testing.T) {
	table, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin() error = %v", err)
	}

	pricing, ok := table.Get("anthropic", "claude-sonnet-4-20250514")
	if !ok {
		t.Fatal("claude-sonnet-4-20250514 not found")
	}

	if pricing.InputPer1M != 3.00 {
		t.Errorf("InputPer1M = %f, want 3.00", pricing.InputPer1M)
	}

	if pricing.OutputPer1M != 15.00 {
		t.Errorf("OutputPer1M = %f, want 15.00", pricing.OutputPer1M)
	}

	if pricing.CacheReadPer1M != 0.30 {
		t.Errorf("CacheReadPer1M = %f, want 0.30", pricing.CacheReadPer1M)
	}
}

func TestGetPricingCaseInsensitive(t *testing.T) {
	table, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin() error = %v", err)
	}

	pricing, ok := table.Get("Anthropic", "Claude-Sonnet-4-20250514")
	if !ok {
		t.Fatal("case insensitive lookup failed")
	}

	if pricing.Provider != "anthropic" {
		t.Errorf("Provider = %s, want anthropic", pricing.Provider)
	}
}

func TestGetPricingNotFound(t *testing.T) {
	table, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin() error = %v", err)
	}

	_, ok := table.Get("nonexistent", "model")
	if ok {
		t.Error("expected not found")
	}
}

func TestComputeCost(t *testing.T) {
	table, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin() error = %v", err)
	}

	cost := table.ComputeCost("anthropic", "claude-sonnet-4-20250514", 1000, 500, 0, 0)

	expectedInput := 1000 * 3.00 / 1_000_000
	expectedOutput := 500 * 15.00 / 1_000_000
	expected := expectedInput + expectedOutput

	if math.Abs(cost-expected) > 0.0001 {
		t.Errorf("ComputeCost() = %f, want %f", cost, expected)
	}
}

func TestComputeCostWithCache(t *testing.T) {
	table, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin() error = %v", err)
	}

	cost := table.ComputeCost("anthropic", "claude-sonnet-4-20250514", 1000, 500, 200, 100)

	expectedInput := 1000 * 3.00 / 1_000_000
	expectedOutput := 500 * 15.00 / 1_000_000
	expectedCacheRead := 200 * 0.30 / 1_000_000
	expectedCacheWrite := 100 * 3.75 / 1_000_000
	expected := expectedInput + expectedOutput + expectedCacheRead + expectedCacheWrite

	if math.Abs(cost-expected) > 0.0001 {
		t.Errorf("ComputeCost() = %f, want %f", cost, expected)
	}
}

func TestComputeCostSimple(t *testing.T) {
	table, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin() error = %v", err)
	}

	cost := table.ComputeCostSimple("openai", "gpt-4o", 10000, 1000)

	expectedInput := 10000 * 2.50 / 1_000_000
	expectedOutput := 1000 * 10.00 / 1_000_000
	expected := expectedInput + expectedOutput

	if math.Abs(cost-expected) > 0.0001 {
		t.Errorf("ComputeCostSimple() = %f, want %f", cost, expected)
	}
}

func TestComputeCostUnknownModel(t *testing.T) {
	table, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin() error = %v", err)
	}

	cost := table.ComputeCost("unknown", "unknown-model", 1000, 500, 0, 0)

	if cost != 0 {
		t.Errorf("ComputeCost() for unknown model = %f, want 0", cost)
	}
}

func TestSetPricing(t *testing.T) {
	table := NewPricingTable()

	table.Set(ModelPricing{
		Provider:    "custom",
		Model:       "custom-model",
		InputPer1M:  5.00,
		OutputPer1M: 20.00,
	})

	pricing, ok := table.Get("custom", "custom-model")
	if !ok {
		t.Fatal("custom model not found")
	}

	if pricing.InputPer1M != 5.00 {
		t.Errorf("InputPer1M = %f, want 5.00", pricing.InputPer1M)
	}
}

func TestLoadWithOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	overridesPath := filepath.Join(tmpDir, "overrides.yaml")

	overridesContent := `
providers:
  anthropic:
    models:
      claude-sonnet-4-20250514:
        input_per_1m: 2.50
        output_per_1m: 12.50
  custom:
    models:
      my-model:
        input_per_1m: 1.00
        output_per_1m: 5.00
`
	if err := os.WriteFile(overridesPath, []byte(overridesContent), 0644); err != nil {
		t.Fatalf("writing overrides: %v", err)
	}

	table, err := LoadWithOverrides(overridesPath)
	if err != nil {
		t.Fatalf("LoadWithOverrides() error = %v", err)
	}

	pricing, ok := table.Get("anthropic", "claude-sonnet-4-20250514")
	if !ok {
		t.Fatal("claude-sonnet-4-20250514 not found")
	}

	if pricing.InputPer1M != 2.50 {
		t.Errorf("InputPer1M = %f, want 2.50 (overridden)", pricing.InputPer1M)
	}

	customPricing, ok := table.Get("custom", "my-model")
	if !ok {
		t.Fatal("custom model not found")
	}

	if customPricing.InputPer1M != 1.00 {
		t.Errorf("custom InputPer1M = %f, want 1.00", customPricing.InputPer1M)
	}

	gptPricing, ok := table.Get("openai", "gpt-4o")
	if !ok {
		t.Fatal("gpt-4o not found (should still be from builtin)")
	}

	if gptPricing.InputPer1M != 2.50 {
		t.Errorf("gpt-4o InputPer1M = %f, want 2.50", gptPricing.InputPer1M)
	}
}

func TestLoadWithOverridesNoFile(t *testing.T) {
	table, err := LoadWithOverrides("/nonexistent/path.yaml")
	if err != nil {
		t.Fatalf("LoadWithOverrides() error = %v", err)
	}

	_, ok := table.Get("anthropic", "claude-sonnet-4-20250514")
	if !ok {
		t.Error("builtin pricing should still be loaded when overrides file doesn't exist")
	}
}

func TestList(t *testing.T) {
	table, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin() error = %v", err)
	}

	list := table.List()
	if len(list) == 0 {
		t.Error("List() returned empty")
	}

	found := false
	for _, entry := range list {
		if entry.Provider == "anthropic" && entry.Model == "claude-sonnet-4-20250514" {
			found = true
			break
		}
	}

	if !found {
		t.Error("claude-sonnet-4-20250514 not in list")
	}
}

func TestModelsForProvider(t *testing.T) {
	table, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin() error = %v", err)
	}

	models := table.ModelsForProvider("anthropic")
	if len(models) == 0 {
		t.Error("no models found for anthropic")
	}

	for _, m := range models {
		if m.Provider != "anthropic" {
			t.Errorf("model.Provider = %s, want anthropic", m.Provider)
		}
	}
}

func TestRealWorldCostCalculation(t *testing.T) {
	table, err := LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin() error = %v", err)
	}

	cost := table.ComputeCost("anthropic", "claude-sonnet-4-20250514", 50000, 10000, 0, 0)
	expectedCost := (50000*3.00 + 10000*15.00) / 1_000_000

	if math.Abs(cost-expectedCost) > 0.001 {
		t.Errorf("cost = $%.4f, want $%.4f", cost, expectedCost)
	}

	t.Logf("50K input + 10K output tokens on Claude Sonnet: $%.4f", cost)
}
