package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/pmontp19/bonpreu-cli/internal/api"
)

func newFavoritesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "favorites",
		Short:         `Starred products ("Preferits")`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newFavoritesListCmd())
	return cmd
}

func newFavoritesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "list",
		Short:         "List favorited products",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			items, err := api.GetFavorites(ctx, rt.client)
			if err != nil {
				return err
			}
			if rt.json {
				return printJSON(items)
			}
			if len(items) == 0 {
				fmt.Fprintln(os.Stderr, "no favorited products")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "UUID\tPRICE\tNAME")
			for _, it := range items {
				fmt.Fprintf(w, "%s\t%s\t%s\n", short(it.ProductID), priceStr(it.Price), it.Name)
			}
			return w.Flush()
		},
	}
}
