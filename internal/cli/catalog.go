package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pmontp19/bonpreu-cli/internal/api"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func newProductCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "product <id>",
		Short:         "Show a product by retailerId or uuid",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			cache, _ := config.LoadCache()
			uuid, err := api.ResolveProductID(ctx, rt.client, args[0], cache)
			if err != nil {
				return err
			}
			prods, err := api.GetProducts(ctx, rt.client, []string{uuid})
			if err != nil {
				return err
			}
			if len(prods) == 0 {
				return fmt.Errorf("no product data for %s", args[0])
			}
			p := prods[0]
			if rt.json {
				return printJSON(productToJSON(p))
			}
			fmt.Printf("%s\n", p.Name)
			if p.Brand != "" {
				fmt.Printf("brand:   %s\n", p.Brand)
			}
			fmt.Printf("ids:     retailer=%s uuid=%s\n", p.RetailerProductID, p.ProductID)
			if p.Price != nil {
				fmt.Printf("price:   %s %s\n", p.Price.Amount, p.Price.Currency)
			}
			return nil
		},
	}
}

func newCategoriesCmd() *cobra.Command {
	var depth int
	cmd := &cobra.Command{
		Use:           "categories",
		Short:         "List the category tree",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			cats, err := api.GetCategories(ctx, rt.client, depth)
			if err != nil {
				return err
			}
			if rt.json {
				return printJSON(cats)
			}
			printCategories(cats, 0)
			return nil
		},
	}
	cmd.Flags().IntVarP(&depth, "depth", "d", 4, "category tree depth")
	return cmd
}

func printCategories(cats []api.Category, indent int) {
	for _, c := range cats {
		fmt.Fprintf(os.Stdout, "%s%s (%d)\n", strings.Repeat("  ", indent), c.Name, c.ProductCount)
		if len(c.ChildCategories) > 0 {
			printCategories(c.ChildCategories, indent+1)
		}
	}
}

func newRelatedCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "related <retailerId>",
		Short:         "List related product uuids for a retailerProductId",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			uuids, err := api.GetRelated(ctx, rt.client, args[0])
			if err != nil {
				return err
			}
			if rt.json {
				return printJSON(uuids)
			}
			for _, u := range uuids {
				fmt.Fprintln(os.Stdout, u)
			}
			return nil
		},
	}
}
