package llm

import (
	"strconv"

	"leadecho/internal/ai"
)

// modelPrices are list prices in USD per 1M tokens {input, output}.
// ponytail: Claude models we route to; others record 0 until added here.
var modelPrices = map[string][2]float64{
	"claude-opus-5-5":             {4, 20},
	"claude-sonnet-5-5":           {2, 10},
	"claude-haiku-5-5":            {0.10, 0.50},
	"anthropic/claude-opus-5.5":   {4, 20},
	"anthropic/claude-sonnet-5.5": {2, 10},
	"anthropic/claude-haiku-5.5":  {0.10, 0.50},
}

// estimateCostUSD prices one call's usage; "0" when the model or usage is unknown.
func estimateCostUSD(model string, u ai.Usage) string {
	p, ok := modelPrices[model]
	if !ok || (u.PromptTokens == 0 && u.CompletionTokens == 0 && u.CacheReadTokens == 0 && u.CacheWriteTokens == 0) {
		return "0"
	}
	// Cache reads bill at 0.1x input, 5-minute cache writes at 1.25x.
	in := float64(u.PromptTokens) + 0.1*float64(u.CacheReadTokens) + 1.25*float64(u.CacheWriteTokens)
	cost := (in*p[0] + float64(u.CompletionTokens)*p[1]) / 1e6
	return strconv.FormatFloat(cost, 'f', 6, 64)
}
