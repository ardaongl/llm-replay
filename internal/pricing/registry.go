package pricing

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed default.yaml
var defaultRegistry []byte

type Rate struct {
	InputPerMillion  float64 `json:"input_per_million" yaml:"input_per_million"`
	OutputPerMillion float64 `json:"output_per_million" yaml:"output_per_million"`
}

type Registry struct {
	Version string          `json:"version" yaml:"version"`
	Models  map[string]Rate `json:"models" yaml:"models"`
}

func Parse(data []byte) (Registry, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var registry Registry
	if err := decoder.Decode(&registry); err != nil {
		return Registry{}, fmt.Errorf("decode pricing registry: %w", err)
	}
	if err := registry.Validate(); err != nil {
		return Registry{}, err
	}
	return registry, nil
}

func Load(path string) (Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Registry{}, fmt.Errorf("read pricing registry: %w", err)
	}
	return Parse(data)
}

func Default() Registry {
	registry, err := Parse(defaultRegistry)
	if err != nil {
		panic(fmt.Sprintf("invalid embedded pricing registry: %v", err))
	}
	return registry
}

// LoadWithDefaults returns embedded prices with optional file-based overrides.
func LoadWithDefaults(path string) (Registry, error) {
	base := Default()
	if path == "" {
		return base, nil
	}
	override, err := Load(path)
	if err != nil {
		return Registry{}, err
	}
	return Merge(base, override), nil
}

func Merge(base, override Registry) Registry {
	merged := Registry{
		Version: base.Version,
		Models:  make(map[string]Rate, len(base.Models)+len(override.Models)),
	}
	for model, rate := range base.Models {
		merged.Models[model] = rate
	}
	for model, rate := range override.Models {
		merged.Models[model] = rate
	}
	if override.Version != "" {
		merged.Version = override.Version
	}
	return merged
}

func (r Registry) Validate() error {
	if strings.TrimSpace(r.Version) == "" {
		return errors.New("pricing version is required")
	}
	if len(r.Models) == 0 {
		return errors.New("pricing models cannot be empty")
	}
	for model, rate := range r.Models {
		if strings.TrimSpace(model) == "" {
			return errors.New("pricing model name cannot be empty")
		}
		if rate.InputPerMillion < 0 || rate.OutputPerMillion < 0 ||
			math.IsNaN(rate.InputPerMillion) || math.IsNaN(rate.OutputPerMillion) ||
			math.IsInf(rate.InputPerMillion, 0) || math.IsInf(rate.OutputPerMillion, 0) {
			return fmt.Errorf("pricing rates for %q must be finite non-negative numbers", model)
		}
	}
	return nil
}

func (r Registry) Cost(model string, inputTokens, outputTokens int64) (float64, error) {
	if inputTokens < 0 || outputTokens < 0 {
		return 0, errors.New("token counts cannot be negative")
	}
	rate, ok := r.Models[model]
	if !ok {
		return 0, fmt.Errorf("pricing not found for model %q", model)
	}
	return float64(inputTokens)/1_000_000*rate.InputPerMillion +
		float64(outputTokens)/1_000_000*rate.OutputPerMillion, nil
}
