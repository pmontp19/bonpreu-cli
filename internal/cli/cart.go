package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/pmontp19/bonpreu-cli/internal/api"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func newCartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "cart",
		Short:         "Cart operations (get/add/remove/set/clear)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newCartGetCmd(), newCartAddCmd(), newCartAddManyCmd(), newCartRemoveCmd(), newCartSetCmd(), newCartClearCmd())
	return cmd
}

// enrichNames fills in Name for lines the cart endpoint returned without one
// (which is the common case — /carts/active doesn't carry product names) via
// a single batched GetProducts call. Best-effort: on any failure it returns
// lines unchanged, so callers fall back to printing the UUID.
func enrichNames(ctx context.Context, rt runtime, lines []api.CartItem) []api.CartItem {
	var missing []string
	for _, it := range lines {
		if it.Name == "" {
			missing = append(missing, it.ProductID)
		}
	}
	if len(missing) == 0 {
		return lines
	}
	prods, err := api.GetProducts(ctx, rt.client, missing)
	if err != nil {
		return lines
	}
	names := make(map[string]string, len(prods))
	for _, p := range prods {
		names[p.ProductID] = p.Name
	}
	for i := range lines {
		if lines[i].Name == "" {
			if n := names[lines[i].ProductID]; n != "" {
				lines[i].Name = n
			}
		}
	}
	return lines
}

func printCart(ctx context.Context, rt runtime, cart *api.Cart) error {
	lines := enrichNames(ctx, rt, cart.Lines())
	if rt.json {
		return printJSON(struct {
			Items int            `json:"items"`
			Total string         `json:"total"`
			Lines []api.CartItem `json:"lines"`
		}{Items: len(lines), Total: cart.TotalAmount(), Lines: lines})
	}
	if len(lines) == 0 {
		fmt.Fprintln(os.Stderr, "cart is empty")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "QTY\tUUID\tPRICE\tNAME")
	for _, it := range lines {
		name := it.Name
		if name == "" {
			name = it.ProductID
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", it.Quantity, short(it.ProductID), priceStr(it.Price), name)
	}
	_ = w.Flush()
	fmt.Fprintf(os.Stdout, "total: %s €\n", cart.TotalAmount())
	return nil
}

func newCartGetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "get", Short: "Show the active cart", SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			cart, err := api.GetActiveCart(ctx, rt.client)
			if err != nil {
				return err
			}
			return printCart(ctx, rt, cart)
		},
	}
}

func newCartAddCmd() *cobra.Command {
	return &cobra.Command{
		Use: "add <id> [qty]", Short: "Add qty (default 1) of a product to the cart",
		Args: cobra.MinimumNArgs(1), SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateDelta(cmd, args, +1)
		},
	}
}

func newCartRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use: "remove <id> [qty]", Short: "Remove qty (default 1) of a product from the cart",
		Args: cobra.MinimumNArgs(1), SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateDelta(cmd, args, -1)
		},
	}
}

func mutateDelta(cmd *cobra.Command, args []string, sign int) error {
	ctx := cmd.Context()
	rt := ctxValue(ctx)
	cache, _ := config.LoadCache()
	uuid, err := api.ResolveProductID(ctx, rt.client, args[0], cache)
	if err != nil {
		return err
	}
	qty := 1
	if len(args) > 1 {
		if qty, err = strconv.Atoi(args[1]); err != nil || qty < 1 {
			return fmt.Errorf("qty must be a positive integer")
		}
	}
	items := []api.CartItemInput{{ProductID: uuid, Quantity: sign * qty}}
	if sign > 0 {
		g, err := rt.guard()
		if err != nil {
			return err
		}
		if err := g.enforceAdd(ctx, rt, items); err != nil {
			return err
		}
	}
	cart, err := api.ApplyQuantity(ctx, rt.client, items)
	if err != nil {
		return err
	}
	return printCart(ctx, rt, cart)
}

func newCartSetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "set <id> <qty>", Short: "Set the absolute quantity of a product",
		Args: cobra.ExactArgs(2), SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			cache, _ := config.LoadCache()
			uuid, err := api.ResolveProductID(ctx, rt.client, args[0], cache)
			if err != nil {
				return err
			}
			target, err := strconv.Atoi(args[1])
			if err != nil || target < 0 {
				return fmt.Errorf("qty must be a non-negative integer")
			}
			cart, err := api.GetActiveCart(ctx, rt.client)
			if err != nil {
				return err
			}
			delta := target - cart.QtyOf(uuid)
			if delta == 0 {
				return printCart(ctx, rt, cart)
			}
			items := []api.CartItemInput{{ProductID: uuid, Quantity: delta}}
			if delta > 0 {
				g, err := rt.guard()
				if err != nil {
					return err
				}
				if err := g.enforceAdd(ctx, rt, items); err != nil {
					return err
				}
			}
			cart, err = api.ApplyQuantity(ctx, rt.client, items)
			if err != nil {
				return err
			}
			return printCart(ctx, rt, cart)
		},
	}
}

func newCartAddManyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "add-many [-f file|-]",
		Short:         "Add multiple products from JSON-lines (id + qty per line)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			cache, _ := config.LoadCache()

			file, _ := cmd.Flags().GetString("file")
			r := os.Stdin
			if file != "" && file != "-" {
				f, err := os.Open(file)
				if err != nil {
					return err
				}
				defer f.Close()
				r = f
			}
			items, err := readItemsJSONLines(r, ctx, rt, cache)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				return fmt.Errorf("no items read")
			}
			g, err := rt.guard()
			if err != nil {
				return err
			}
			if err := g.enforceAdd(ctx, rt, items); err != nil {
				return err
			}
			cart, err := api.ApplyQuantity(ctx, rt.client, items)
			if err != nil {
				return err
			}
			return printCart(ctx, rt, cart)
		},
	}
	cmd.Flags().StringP("file", "f", "", "JSON-lines file (each line {\"id\",\"qty\"}); '-' or omit for stdin")
	return cmd
}

func readItemsJSONLines(r *os.File, ctx context.Context, rt runtime, cache *config.IDCache) ([]api.CartItemInput, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var items []api.CartItemInput
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry struct {
			ID  string `json:"id"`
			Qty int    `json:"qty"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("parse line %q: %w", line, err)
		}
		if entry.ID == "" {
			return nil, fmt.Errorf("line missing \"id\": %s", line)
		}
		if entry.Qty <= 0 {
			entry.Qty = 1
		}
		uuid, err := api.ResolveProductID(ctx, rt.client, entry.ID, cache)
		if err != nil {
			return nil, err
		}
		items = append(items, api.CartItemInput{ProductID: uuid, Quantity: entry.Qty})
	}
	return items, scanner.Err()
}

func newCartClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "clear",
		Short:         "Remove all items from the cart",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			cart, err := api.GetActiveCart(ctx, rt.client)
			if err != nil {
				return err
			}
			lines := cart.Lines()
			if len(lines) == 0 {
				return printCart(ctx, rt, cart)
			}
			items := make([]api.CartItemInput, 0, len(lines))
			for _, it := range lines {
				items = append(items, api.CartItemInput{ProductID: it.ProductID, Quantity: -it.Quantity})
			}
			cart, err = api.ApplyQuantity(ctx, rt.client, items)
			if err != nil {
				return err
			}
			return printCart(ctx, rt, cart)
		},
	}
}
