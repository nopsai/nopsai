package config

import "testing"

func floatPtr(value float64) *float64 { return &value }

func TestLLMPricingCostUSDBillsCacheTokensAtTheirOwnRates(t *testing.T) {
	pricing := LLMPricing{
		InputPerMillionUSD:       3,
		OutputPerMillionUSD:      15,
		CachedInputPerMillionUSD: floatPtr(0.30),
		CacheWritePerMillionUSD:  floatPtr(3.75),
	}
	// 1,000,000 prompt tokens: 100k written to cache, 800k read from cache,
	// 100k charged at the standard input rate.
	input, output := pricing.CostUSD(1_000_000, 200_000, 800_000, 100_000)

	// 800k cache reads at $0.30/M, 100k cache writes at $3.75/M, 100k uncached
	// input at $3.00/M.
	wantInput := 0.24 + 0.375 + 0.30
	if !floatsClose(input, wantInput) {
		t.Errorf("input cost = %g, want %g", input, wantInput)
	}
	wantOutput := 3.0
	if !floatsClose(output, wantOutput) {
		t.Errorf("output cost = %g, want %g", output, wantOutput)
	}
}

func TestLLMPricingCostUSDDefaultsCacheRatesToInputRate(t *testing.T) {
	pricing := LLMPricing{InputPerMillionUSD: 2, OutputPerMillionUSD: 6}
	if got := pricing.CachedInputRate(); got != 2 {
		t.Errorf("CachedInputRate() = %g, want 2 when the provider does not discount cache reads", got)
	}
	if got := pricing.CacheWriteRate(); got != 2 {
		t.Errorf("CacheWriteRate() = %g, want 2", got)
	}
	input, _ := pricing.CostUSD(1_000_000, 0, 500_000, 0)
	if !floatsClose(input, 2) {
		t.Errorf("input cost = %g, want 2; undiscounted cache reads bill at the input rate", input)
	}
}

// A provider that reports more cached tokens than prompt tokens must not drive
// the uncached remainder negative and refund the caller.
func TestLLMPricingCostUSDFloorsUncachedRemainderAtZero(t *testing.T) {
	pricing := LLMPricing{InputPerMillionUSD: 10, OutputPerMillionUSD: 10, CachedInputPerMillionUSD: floatPtr(1)}
	input, _ := pricing.CostUSD(1_000, 0, 5_000, 0)
	if input < 0 {
		t.Fatalf("input cost = %g, want a non-negative figure", input)
	}
	if !floatsClose(input, 0.005) {
		t.Errorf("input cost = %g, want 0.005 (cached tokens only)", input)
	}
}

func TestLLMPricingCostUSDIsZeroForAFreeModel(t *testing.T) {
	pricing := LLMPricing{}
	input, output := pricing.CostUSD(500_000, 500_000, 0, 0)
	if input != 0 || output != 0 {
		t.Errorf("cost = (%g, %g), want (0, 0) for an explicitly free model", input, output)
	}
}

func floatsClose(got, want float64) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-9
}
