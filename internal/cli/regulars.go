package cli

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/pmontp19/bonpreu-cli/internal/api"
)

func newRegularsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "regulars",
		Short:         `Frequently-bought products ("Productes recurrents" / "Compra ràpida")`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newRegularsListCmd(), newRegularsFillCmd())
	return cmd
}

func newRegularsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "list",
		Short:         "List frequently-bought products (the source `regulars fill` draws from)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			items, err := api.GetRegulars(ctx, rt.client)
			if err != nil {
				return err
			}
			if rt.json {
				return printJSON(items)
			}
			if len(items) == 0 {
				fmt.Fprintln(os.Stderr, "no regular products")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "QTY\tFREQUENCY\tPRICE\tNAME")
			for _, it := range items {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", it.Quantity, it.Frequency, priceStr(it.Price), it.Name)
			}
			return w.Flush()
		},
	}
}

func newRegularsFillCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "fill",
		Short:         `Auto-fill the cart from purchase history ("Compra ràpida" / smart shop)`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			g, err := rt.guard()
			if err != nil {
				return err
			}
			res, err := api.InstantShop(ctx, rt.client)
			if err != nil {
				return err
			}
			// Unlike cart add/set, instant-shop picks products server-side —
			// there's no delta to check against --max before the call. Enforce
			// the guard after the fact instead: if the resulting total breaks
			// the cap, roll back exactly what was just added.
			if g.enabled() {
				total, perr := strconv.ParseFloat(res.Total, 64)
				if perr != nil {
					return fmt.Errorf("spending guard: unreadable total %q after instant-shop (not rolled back — check `cart get`)", res.Total)
				}
				if total > g.max {
					rollback := make([]api.CartItemInput, 0, len(res.AddedProducts))
					for _, it := range res.AddedProducts {
						rollback = append(rollback, api.CartItemInput{ProductID: it.ProductID, Quantity: -it.Quantity})
					}
					if _, rerr := api.ApplyQuantity(ctx, rt.client, rollback); rerr != nil {
						return fmt.Errorf("spending guard: total %.2f > --max %.2f, and rollback failed: %w (cart left mutated, check `cart get`)", total, g.max, rerr)
					}
					return fmt.Errorf("spending guard: instant-shop would total %.2f > --max %.2f (added items rolled back)", total, g.max)
				}
			}
			if rt.json {
				return printJSON(res)
			}
			fmt.Fprintf(os.Stdout, "added %d products, cart total: %s €\n", len(res.AddedProducts), res.Total)
			return nil
		},
	}
}
