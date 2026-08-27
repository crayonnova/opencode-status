package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Model struct {
	ID        string `json:"id"`
	Provider  string `json:"provider_id"`
	Name      string `json:"name"`
	IsFree    bool   `json:"is_free"`
	InputCost float64 `json:"cost_input"`
	OutputCost float64 `json:"cost_output"`
}

// models.dev top-level shape: { "<provider>": { ..., "models": { "<model>": { ... } } } }
type modelsDevProvider struct {
	Models map[string]modelsDevModel `json:"models"`
}
type modelsDevModel struct {
	Name string         `json:"name"`
	Cost modelsDevCost  `json:"cost"`
}
type modelsDevCost struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// OpenRouter /api/v1/models response: { "data": [ { "id": ..., "pricing": { "prompt": "...", "completion": "..." } } ] }
type openRouterResponse struct {
	Data []openRouterModel `json:"data"`
}
type openRouterModel struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Pricing openRouterPricing  `json:"pricing"`
}
type openRouterPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Fetcher struct {
	Client       HTTPDoer
	ModelsDevURL string
	OpenRouterURL string
}

func New(modelsDevURL, openRouterURL string) *Fetcher {
	return &Fetcher{
		Client:        &http.Client{Timeout: 30 * time.Second},
		ModelsDevURL:  modelsDevURL,
		OpenRouterURL: openRouterURL,
	}
}

// FetchFromModelsDev pulls the entire models.dev catalog and returns all free models (cost == 0).
func (f *Fetcher) FetchFromModelsDev(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.ModelsDevURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "opencode-status/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("models.dev request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("models.dev status %d: %s", resp.StatusCode, string(body))
	}

	var raw map[string]modelsDevProvider
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("models.dev decode: %w", err)
	}

	var out []Model
	for provID, prov := range raw {
		for modelID, m := range prov.Models {
			// Free = both input and output cost are 0.
			if m.Cost.Input == 0 && m.Cost.Output == 0 {
				out = append(out, Model{
					ID:         provID + "/" + modelID,
					Provider:   provID,
					Name:       firstNonEmpty(m.Name, modelID),
					IsFree:     true,
					InputCost:  m.Cost.Input,
					OutputCost: m.Cost.Output,
				})
			}
		}
	}
	return out, nil
}

// FetchFromOpenRouter pulls the live OpenRouter model list and returns those ending in ":free"
// or with $0 prompt+completion price. Includes both free and paid when includePaid.
func (f *Fetcher) FetchFromOpenRouter(ctx context.Context, includePaid bool) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.OpenRouterURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "opencode-status/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("openrouter status %d: %s", resp.StatusCode, string(body))
	}

	var data openRouterResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&data); err != nil {
		return nil, fmt.Errorf("openrouter decode: %w", err)
	}

	var out []Model
	seen := map[string]bool{}
	for _, m := range data.Data {
		isFree := strings.HasSuffix(m.ID, ":free")
		if !includePaid && !isFree {
			continue
		}
		// Build a stable provider from id prefix.
		provID := m.ID
		if idx := strings.Index(m.ID, "/"); idx > 0 {
			provID = m.ID[:idx]
		}
		key := m.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Model{
			ID:      m.ID,
			Provider: provID,
			Name:    firstNonEmpty(m.Name, m.ID),
			IsFree:  isFree,
		})
	}
	return out, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
