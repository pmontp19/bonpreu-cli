package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pmontp19/bonpreu-cli/internal/api"
	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func newImportHarCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import-har --file <path>",
		Short: "Parse a HAR export and save the Bonpreu session",
		Long: `Parse a browser HAR file (exported after logging in to compraonline.bonpreuesclat.cat)
and extract the session cookies, CSRF token, region and delivery-destination defaults.
The HAR is read once and never stored; only the derived session is written to ~/.bonpreu/ (0600).`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("file")
			if path == "" {
				return fmt.Errorf("--file is required")
			}
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("open har: %w", err)
			}
			defer f.Close()
			sess, err := client.ParseSession(f)
			if err != nil {
				return fmt.Errorf("parse har: %w", err)
			}
			if err := config.SaveSession(sess); err != nil {
				return err
			}
			if f := FromContext(cmd.Context()).Flags; f != nil && f.JSON {
				return printJSON(sessionSummary(sess))
			}
			fmt.Fprintf(os.Stderr, "session saved to ~/.bonpreu (region=%s dest=%s cookies=%d csrf=%s)\n",
				sess.RegionID, sess.DeliveryDestinationID, len(sess.Cookies), maskUUID(sess.CSRFToken))
			return nil
		},
	}
	cmd.Flags().StringP("file", "f", "", "path to the exported .har file (required)")
	return cmd
}

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "whoami",
		Short:         "Verify the saved session by fetching the active cart",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c := ctxValue(ctx)
			cart, err := api.GetActiveCart(ctx, c.client)
			if err != nil {
				return fmt.Errorf("session check failed (re-run import-har if expired): %w", err)
			}
			lines := cart.Lines()
			summary := struct {
				Items int    `json:"items"`
				Total string `json:"total"`
			}{Items: len(lines), Total: cart.TotalAmount()}
			if ctxValue(ctx).json {
				return printJSON(summary)
			}
			fmt.Printf("session OK — %d items, total %s €\n", summary.Items, summary.Total)
			return nil
		},
	}
}

type runtime struct {
	client *client.Client
	flags  *Flags
	json   bool
	log    func(format string, a ...any)
}

func ctxValue(ctx context.Context) runtime {
	if h := holderFrom(ctx); h != nil && h.rt != nil {
		return *h.rt
	}
	from := FromContext(ctx)
	sess, err := config.LoadSession()
	if err != nil {
		sess = &config.Session{}
	}
	var c *client.Client
	if from.Logger != nil {
		c, err = client.New(sess, from.Logger)
	} else {
		c, err = client.New(sess, nil)
	}
	if err != nil {
		c, _ = client.New(sess, nil)
	}
	var f *Flags
	if from.Flags != nil {
		f = from.Flags
	}
	rt := runtime{
		client: c,
		flags:  f,
		json:   f != nil && f.JSON,
		log: func(format string, a ...any) {
			if from.Logger != nil {
				from.Logger.Printf(format, a...)
			}
		},
	}
	if h := holderFrom(ctx); h != nil {
		h.rt = &rt
	}
	return rt
}

// sessionSummary is the sanitized, machine-readable view of an imported
// session — counts and defaults only, never cookie values or the CSRF token.
func sessionSummary(s *config.Session) map[string]any {
	return map[string]any{
		"region":         s.RegionID,
		"dest":           s.DeliveryDestinationID,
		"cookies":        len(s.Cookies),
		"has_csrf":       s.CSRFToken != "",
		"source_version": s.EcomRequestSourceVersion,
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func maskUUID(s string) string {
	if len(s) < 9 {
		return s
	}
	return s[:8] + "…"
}
