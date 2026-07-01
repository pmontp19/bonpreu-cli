package cli

import (
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/pmontp19/bonpreu-cli/internal/config"
)

type Flags struct {
	JSON    bool
	Config  string
	Max     float64
	Verbose bool
}

func NewRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "bonpreu",
		Short:         "Unofficial CLI for compraonline.bonpreuesclat.cat",
		Long:          "Unofficial, agent-friendly CLI for Bonpreu/Esclat online. Search, cart, and delivery slots with --json output. Order placement (3DS) is done in the web/app.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	f := &Flags{}
	root.PersistentFlags().BoolVar(&f.JSON, "json", false, "machine-readable JSON to stdout")
	root.PersistentFlags().StringVar(&f.Config, "config", "", "config path (default ~/.bonpreu/config.json)")
	root.PersistentFlags().Float64Var(&f.Max, "max", 0, "spending guard: refuse cart mutations above this EUR total (env BONPREU_MAX_EUR)")
	root.PersistentFlags().BoolVarP(&f.Verbose, "verbose", "v", false, "diagnostics to stderr")

	logger := log.New(os.Stderr, "bonpreu: ", log.LstdFlags)
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		cmd.SetContext(WithFlags(cmd.Context(), f, logger))
		return nil
	}
	// After a successful command, fold any rotated CSRF token / refreshed
	// cookies back to disk so the next invocation starts from a live session.
	root.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		h := holderFrom(cmd.Context())
		if h == nil || h.rt == nil || h.rt.client == nil {
			return nil
		}
		if h.rt.client.SyncSession() {
			if err := config.SaveSession(h.rt.client.Sess); err != nil {
				logger.Printf("warning: could not persist session: %v", err)
			}
		}
		return nil
	}
	root.AddCommand(newImportHarCmd(), newWhoamiCmd(), newSearchCmd(),
		newProductCmd(), newCategoriesCmd(), newRelatedCmd(), newCartCmd(),
		newDeliveryCmd(), newSlotsCmd(), newOrdersCmd(), newCheckoutCmd())
	return root
}
