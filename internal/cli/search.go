package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/pmontp19/bonpreu-cli/internal/api"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func newSearchCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:           "search <query>",
		Short:         "Search the catalog (returns products with retailerId + uuid + price)",
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			query := joinArgs(args)
			prods, err := api.SearchProducts(ctx, rt.client, query, limit)
			if err != nil {
				return err
			}
			cacheIDCache(ctx, prods)
			if rt.json {
				return printJSON(productsToJSON(prods))
			}
			if len(prods) == 0 {
				fmt.Fprintln(os.Stderr, "no results")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "RETAILERID\tUUID\tPRICE\tNAME")
			for _, p := range prods {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.RetailerProductID, short(p.ProductID), priceStr(p.Price), p.Name)
			}
			return w.Flush()
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "l", 30, "max results to decorate")
	return cmd
}

func cacheIDCache(ctx context.Context, prods []api.Product) {
	cache, err := config.LoadCache()
	if err != nil || cache == nil {
		return
	}
	changed := false
	for _, p := range prods {
		if p.RetailerProductID != "" && p.ProductID != "" {
			if cache.RetailerToProduct[p.RetailerProductID] != p.ProductID {
				cache.RetailerToProduct[p.RetailerProductID] = p.ProductID
				changed = true
			}
		}
	}
	if changed {
		_ = config.SaveCache(cache)
	}
}

type productJSON struct {
	ID    string `json:"id"`
	UUID  string `json:"uuid"`
	Name  string `json:"name"`
	Brand string `json:"brand,omitempty"`
	Price string `json:"price,omitempty"`
	Unit  string `json:"unit,omitempty"`
}

func productsToJSON(prods []api.Product) []productJSON {
	out := make([]productJSON, 0, len(prods))
	for _, p := range prods {
		out = append(out, productToJSON(p))
	}
	return out
}

func productToJSON(p api.Product) productJSON {
	return productJSON{
		ID: p.RetailerProductID, UUID: p.ProductID, Name: p.Name,
		Brand: p.Brand, Price: priceStr(p.Price), Unit: p.Unit,
	}
}

func joinArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}

func priceStr(m *api.Money) string {
	if m == nil {
		return ""
	}
	return m.Amount
}

func short(s string) string {
	if len(s) > 13 {
		return s[:8] + "…" + s[len(s)-4:]
	}
	return s
}
