package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
			// Read the flag directly rather than via ctxValue(): ctxValue builds a
			// client from the pre-import session and memoizes it, which would let
			// PostRunE's SyncSession clobber the session we just wrote.
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

func newImportCurlCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import-curl [--file <path>|-]",
		Short: "Import a session from a devtools \"Copy as cURL\" command",
		Long: `Capture an authenticated session from a browser "Copy as cURL".

In the devtools Network panel, right-click any request to compraonline.bonpreuesclat.cat
made while logged in → Copy → "Copy as cURL", then pipe or paste it here. Unlike a HAR
export (which recent Chrome sanitizes, stripping the Cookie and x-csrf-token headers),
"Copy as cURL" carries the request verbatim, so it reliably captures the session in one
click.

Reads stdin by default, or --file <path> ("-" is stdin). The input is parsed once and never
stored; only the derived session is written to ~/.bonpreu/ (0600). A missing CSRF token is
derived from the homepage, and the home delivery destination is resolved from the account so
slots work out of the box.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			file, _ := cmd.Flags().GetString("file")
			r := io.Reader(os.Stdin)
			if file != "" && file != "-" {
				f, err := os.Open(file)
				if err != nil {
					return fmt.Errorf("open curl file: %w", err)
				}
				defer f.Close()
				r = f
			}
			sess, err := client.ParseCurl(r)
			if err != nil {
				return fmt.Errorf("parse curl: %w", err)
			}

			ctx := cmd.Context()
			logger := FromContext(ctx).Logger
			// Build the client directly (not via ctxValue) so root's PostRunE
			// SyncSession does not clobber the session we save here.
			c, err := client.New(sess, logger)
			if err != nil {
				return err
			}
			// Derive the CSRF token from the homepage if the copied request had none.
			if sess.CSRFToken == "" {
				if _, err := c.RefreshCSRF(ctx); err != nil {
					return fmt.Errorf("could not derive CSRF token from homepage: %w", err)
				}
			}
			// Refuse to save a session that is not actually logged in.
			if status, serr := api.GetAccountStatus(ctx, c); serr == nil && !status.Authenticated {
				return fmt.Errorf("the copied request is not from a logged-in session " +
					"(homepage reports not logged in) — log in in the browser first, then copy an authenticated request")
			}
			// Best-effort: resolve the account's primary home destination/region so
			// `slots` works without a separate `delivery use`. Non-fatal on failure.
			if sess.DeliveryDestinationID == "" || sess.RegionID == "" {
				if addrs, aerr := api.GetDeliveryAddresses(ctx, c, api.MethodHome); aerr == nil && len(addrs) > 0 {
					best := addrs[0]
					for _, a := range addrs {
						if a.IsPrimary {
							best = a
							break
						}
					}
					sess.DeliveryDestinationID = best.DeliveryDestinationID
					sess.RegionID = best.ResolvedRegionID
				} else if aerr != nil && logger != nil {
					logger.Printf("warning: could not resolve home delivery destination: %v", aerr)
				}
			}
			// Fold any cookies/CSRF refreshed by the requests above into the session.
			c.SyncSession()
			if err := config.SaveSession(sess); err != nil {
				return err
			}
			if f := FromContext(ctx).Flags; f != nil && f.JSON {
				return printJSON(sessionSummary(sess))
			}
			fmt.Fprintf(os.Stderr, "session saved to ~/.bonpreu (region=%s dest=%s cookies=%d csrf=%s)\n",
				sess.RegionID, sess.DeliveryDestinationID, len(sess.Cookies), maskUUID(sess.CSRFToken))
			return nil
		},
	}
	cmd.Flags().StringP("file", "f", "", "file with the curl command (default stdin; \"-\" is stdin)")
	return cmd
}

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "whoami",
		Short:         "Verify the saved session (account auth + active cart)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c := ctxValue(ctx)

			// The guest cart responds even when the account session has lapsed,
			// so checking the cart alone gives a false "OK". Verify account-level
			// auth first via the homepage session.isLoggedIn flag.
			status, serr := api.GetAccountStatus(ctx, c.client)
			if serr == nil && !status.Authenticated {
				return fmt.Errorf("session is anonymous or account auth has expired " +
					"(the guest cart still works, but orders/wallet/slots/delivery do not) — " +
					"re-run `bonpreu import-har --file <fresh.har>` to refresh your session")
			}
			if serr != nil {
				// Could not reach/parse the homepage; fall back to the cart check
				// and note that account auth is unverified.
				c.log("warning: could not verify account auth: %v", serr)
			}

			cart, err := api.GetActiveCart(ctx, c.client)
			if err != nil {
				var he *client.HTTPError
				if errors.As(err, &he) && he.Expired() {
					// HTTPError already carries the actionable re-import message.
					return err
				}
				return fmt.Errorf("session check failed (re-run import-har if expired): %w", err)
			}
			authenticated := serr == nil && status.Authenticated
			summary := struct {
				Authenticated bool   `json:"authenticated"`
				Products      int    `json:"products"` // distinct product lines
				Articles      int    `json:"articles"` // total units (site's "articles" count)
				Total         string `json:"total"`
			}{Authenticated: authenticated, Products: len(cart.Lines()), Articles: cart.TotalUnits(), Total: cart.TotalAmount()}
			if ctxValue(ctx).json {
				return printJSON(summary)
			}
			auth := "authenticated"
			if !authenticated {
				auth = "account auth UNVERIFIED"
			}
			fmt.Printf("session OK (%s) — %d products / %d articles, total %s €\n",
				auth, summary.Products, summary.Articles, summary.Total)
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
