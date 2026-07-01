package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pmontp19/bonpreu-cli/internal/api"
)

func newLoyaltyCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "loyalty",
		Short:         "Show the Guardiola (loyalty wallet) balance",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			bal, err := api.GetLoyaltyBalance(ctx, rt.client)
			if err != nil {
				return err
			}
			if rt.json {
				return printJSON(bal)
			}
			if !bal.Registered {
				fmt.Fprintln(os.Stderr, "not enrolled in the loyalty program")
				return nil
			}
			fmt.Printf("Guardiola balance: %s %s\n", bal.Money.Amount, bal.Money.Currency)
			return nil
		},
	}
}
