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
	cfg, err := loadConfig(rt.flags)
	if err != nil {
		return guard{}, fmt.Errorf("spending guard: could not load config: %w", err)
	}
	if cfg.DefaultMaxEUR > 0 {
		return guard{max: cfg.DefaultMaxEUR}, nil
	}
	return guard{}, nil
}

// loadConfig honors --config when set, so a user pointing the CLI at an
// alternate config.json doesn't silently fall back to the default path.
func loadConfig(f *Flags) (*config.Config, error) {
	if f != nil && f.Config != "" {
		return config.LoadConfigFrom(f.Config)
	}
	return config.LoadConfig()
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
	var uuids []string
	for _, it := range items {
		if it.Quantity > 0 {
			uuids = append(uuids, it.ProductID)
		}
	}
	prices := map[string]float64{}
	if len(uuids) > 0 {
		prods, err := api.GetProducts(ctx, rt.client, uuids)
		if err != nil {
			return fmt.Errorf("spending guard: %w (fail-closed)", err)
		}
		for _, p := range prods {
			if p.Price != nil {
				if v, err := strconv.ParseFloat(p.Price.Amount, 64); err == nil {
					prices[p.ProductID] = v
				}
			}
		}
	}
	added := 0.0
	for _, it := range items {
		if it.Quantity <= 0 {
			continue
		}
		price, ok := prices[it.ProductID]
		if !ok {
			return fmt.Errorf("spending guard: price not found for %s (fail-closed)", it.ProductID)
		}
		added += price * float64(it.Quantity)
	}
	if current+added > g.max {
		return fmt.Errorf("spending guard: cart %.2f + %.2f > --max %.2f (refused)", current, added, g.max)
	}
	return nil
}
