package pricing

import (
	"embed"
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed pricing.yaml
var builtinPricingFS embed.FS

type PricingTable struct {
	mu        sync.RWMutex
	entries   map[string]ModelPricing
	providers map[string]bool
}

type ModelPricing struct {
	Provider        string
	Model           string
	InputPer1M      float64
	OutputPer1M     float64
	CacheReadPer1M  float64
	CacheWritePer1M float64
}

type pricingYAML struct {
	Providers map[string]providerYAML `yaml:"providers"`
}

type providerYAML struct {
	Models map[string]modelYAML `yaml:"models"`
}

type modelYAML struct {
	InputPer1M      float64 `yaml:"input_per_1m"`
	OutputPer1M     float64 `yaml:"output_per_1m"`
	CacheReadPer1M  float64 `yaml:"cache_read_per_1m"`
	CacheWritePer1M float64 `yaml:"cache_write_per_1m"`
}

func NewPricingTable() *PricingTable {
	return &PricingTable{
		entries:   make(map[string]ModelPricing),
		providers: make(map[string]bool),
	}
}

func LoadBuiltin() (*PricingTable, error) {
	data, err := builtinPricingFS.ReadFile("pricing.yaml")
	if err != nil {
		return nil, fmt.Errorf("reading builtin pricing: %w", err)
	}

	table := NewPricingTable()
	if err := table.loadFromYAML(data); err != nil {
		return nil, fmt.Errorf("parsing builtin pricing: %w", err)
	}

	return table, nil
}

func LoadFromFile(path string) (*PricingTable, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading pricing file: %w", err)
	}

	table := NewPricingTable()
	if err := table.loadFromYAML(data); err != nil {
		return nil, fmt.Errorf("parsing pricing file: %w", err)
	}

	return table, nil
}

func LoadWithOverrides(overridesPath string) (*PricingTable, error) {
	table, err := LoadBuiltin()
	if err != nil {
		return nil, err
	}

	if overridesPath == "" {
		return table, nil
	}

	data, err := os.ReadFile(overridesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return table, nil
		}
		return nil, fmt.Errorf("reading overrides file: %w", err)
	}

	if err := table.loadFromYAML(data); err != nil {
		return nil, fmt.Errorf("parsing overrides: %w", err)
	}

	return table, nil
}

func (t *PricingTable) loadFromYAML(data []byte) error {
	var pricing pricingYAML
	if err := yaml.Unmarshal(data, &pricing); err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for provider, prov := range pricing.Providers {
		t.providers[provider] = true
		for model, mp := range prov.Models {
			key := pricingKey(provider, model)
			t.entries[key] = ModelPricing{
				Provider:        provider,
				Model:           model,
				InputPer1M:      mp.InputPer1M,
				OutputPer1M:     mp.OutputPer1M,
				CacheReadPer1M:  mp.CacheReadPer1M,
				CacheWritePer1M: mp.CacheWritePer1M,
			}
		}
	}

	return nil
}

func (t *PricingTable) Get(provider, model string) (ModelPricing, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	key := pricingKey(provider, model)
	entry, ok := t.entries[key]
	return entry, ok
}

func (t *PricingTable) Set(pricing ModelPricing) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := pricingKey(pricing.Provider, pricing.Model)
	t.entries[key] = pricing
	t.providers[pricing.Provider] = true
}

func (t *PricingTable) ComputeCost(provider, model string, inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int) float64 {
	pricing, ok := t.Get(provider, model)
	if !ok {
		return 0
	}

	cost := 0.0
	cost += float64(inputTokens) * pricing.InputPer1M / 1_000_000
	cost += float64(outputTokens) * pricing.OutputPer1M / 1_000_000

	if cacheReadTokens > 0 && pricing.CacheReadPer1M > 0 {
		cost += float64(cacheReadTokens) * pricing.CacheReadPer1M / 1_000_000
	}

	if cacheWriteTokens > 0 && pricing.CacheWritePer1M > 0 {
		cost += float64(cacheWriteTokens) * pricing.CacheWritePer1M / 1_000_000
	}

	return cost
}

func (t *PricingTable) ComputeCostSimple(provider, model string, inputTokens, outputTokens int) float64 {
	return t.ComputeCost(provider, model, inputTokens, outputTokens, 0, 0)
}

func (t *PricingTable) List() []ModelPricing {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]ModelPricing, 0, len(t.entries))
	for _, entry := range t.entries {
		result = append(result, entry)
	}
	return result
}

func (t *PricingTable) Providers() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]string, 0, len(t.providers))
	for p := range t.providers {
		result = append(result, p)
	}
	return result
}

func (t *PricingTable) ModelsForProvider(provider string) []ModelPricing {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []ModelPricing
	for _, entry := range t.entries {
		if entry.Provider == provider {
			result = append(result, entry)
		}
	}
	return result
}

func pricingKey(provider, model string) string {
	return strings.ToLower(provider) + ":" + strings.ToLower(model)
}
