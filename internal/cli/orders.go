package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/pmontp19/bonpreu-cli/internal/api"
)

func newOrdersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "orders",
		Short:         "Read-only order history",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newOrdersListCmd(), newOrdersShowCmd())
	return cmd
}

func newOrdersListCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:           "list",
		Short:         "List prior orders",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			orders, err := api.GetOrders(ctx, rt.client, limit)
			if err != nil {
				return err
			}
			if rt.json {
				return printJSON(orders)
			}
			if len(orders) == 0 {
				fmt.Fprintln(os.Stderr, "no orders")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ORDERID\tSTATUS\tPLACED\tDELIVERY\tTOTAL")
			for _, o := range orders {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					o.OrderID, o.Status, o.PlacedAt, o.DeliveryDate, priceStr(o.Total))
			}
			return w.Flush()
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, "max number of orders (0 = all)")
	return cmd
}

func newOrdersShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "show <orderId>",
		Short:         "Show a single order with its line items",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			ord, err := api.GetOrder(ctx, rt.client, args[0])
			if err != nil {
				return err
			}
			if rt.json {
				return printJSON(ord)
			}
			fmt.Fprintf(os.Stdout, "order:   %s\n", ord.OrderID)
			if ord.Status != "" {
				fmt.Fprintf(os.Stdout, "status:  %s\n", ord.Status)
			}
			if ord.Total != nil && ord.Total.Amount != "" {
				fmt.Fprintf(os.Stdout, "total:   %s %s\n", ord.Total.Amount, ord.Total.Currency)
			}
			if len(ord.Lines) == 0 {
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "QTY\tNAME\tRETAILER\tPRICE")
			for _, l := range ord.Lines {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\n",
					l.Quantity, l.Product.Name, l.Product.RetailerProductID, priceStr(l.Price))
			}
			return w.Flush()
		},
	}
}
