package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/pmontp19/bonpreu-cli/internal/api"
)

func newWalletCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "wallet",
		Short:         "Read-only saved payment methods",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newWalletListCmd())
	return cmd
}

func newWalletListCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "list",
		Short:         "List saved payment methods",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			methods, err := api.GetWalletItems(ctx, rt.client)
			if err != nil {
				return err
			}
			if rt.json {
				return printJSON(methods)
			}
			if len(methods) == 0 {
				fmt.Fprintln(os.Stderr, "no saved payment methods")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tCARD\tEXPIRES\tDEFAULT\tEXPIRED")
			for _, m := range methods {
				fmt.Fprintf(w, "%s\t%s **** %s\t%s/%s\t%t\t%t\n",
					m.WalletItemID, m.CardType, m.LastFourDigits, m.ExpiryMonth, m.ExpiryYear, m.Default, m.Expired)
			}
			return w.Flush()
		},
	}
}
