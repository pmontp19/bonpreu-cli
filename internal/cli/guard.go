package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/pmontp19/bonpreu-cli/internal/api"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

type guard struct {
	max float64
}

// guard resolves the spending cap by precedence: --max flag, then
// BONPREU_MAX_EUR, then config.json's default_max_eur. A malformed env value is
// an error rather than a silently disabled guard.
func (rt runtime) guard() (guard, error) {
	if rt.flags != nil && rt.flags.Max > 0 {
		return guard{max: rt.flags.Max}, nil
	}
	if env := os.Getenv("BONPREU_MAX_EUR"); env != "" {
		v, err := strconv.ParseFloat(env, 64)
		if err != nil {
			return guard{}, fmt.Errorf("BONPREU_MAX_EUR=%q is not a number: %w", env, err)
		}
		return guard{max: v}, nil
	}
	if cfg, err := config.LoadConfig(); err == nil && cfg.DefaultMaxEUR > 0 {
		return guard{max: cfg.DefaultMaxEUR}, nil
	}
	return guard{}, nil
}

func (g guard) enabled() bool { return g.max > 0 }

func (g guard) enforceAdd(ctx context.Context, rt runtime, items []api.CartItemInput) error {
	if !g.enabled() {
		return nil
	}
	cart, err := api.GetActiveCart(ctx, rt.client)
	if err != nil {
		return fmt.Errorf("spending guard: cannot read cart total (fail-closed): %w", err)
	}
	current, err := strconv.ParseFloat(cart.TotalAmount(), 64)
	if err != nil {
		return fmt.Errorf("spending guard: unreadable cart total %q (fail-closed)", cart.TotalAmount())
	}
	added := 0.0
	for _, it := range items {
		if it.Quantity <= 0 {
			continue
		}
		p, err := api.PriceOf(ctx, rt.client, it.ProductID)
		if err != nil {
			return fmt.Errorf("spending guard: %w (fail-closed)", err)
		}
		added += p * float64(it.Quantity)
	}
	if current+added > g.max {
		return fmt.Errorf("spending guard: cart %.2f + %.2f > --max %.2f (refused)", current, added, g.max)
	}
	return nil
}
