package cli

import (
	"fmt"
	"os"
	"os/exec"
	goruntime "runtime"

	"github.com/spf13/cobra"

	"github.com/pmontp19/bonpreu-cli/internal/client"
)

func newCheckoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "checkout",
		Short:         "Checkout handoff (order placement happens in the browser)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newCheckoutOpenCmd())
	return cmd
}

func newCheckoutOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "open",
		Short:         "Open the default browser at /checkout to finish the order (3DS)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			target := client.BaseURL + "/checkout"
			name, argv, err := browserOpenArgs(goruntime.GOOS, target)
			if err != nil {
				return err
			}
			if err := execStart(name, argv); err != nil {
				return fmt.Errorf("open browser: %w", err)
			}
			if rt.json {
				return printJSON(map[string]string{"url": target})
			}
			fmt.Fprintf(os.Stdout, "opening %s\n", target)
			return nil
		},
	}
}

// execStart launches the platform's open command; overridden in tests so
// `checkout open --json` can be exercised without popping a real browser.
var execStart = func(name string, argv []string) error {
	return exec.Command(name, argv...).Start()
}

// browserOpenArgs returns the command and arguments that open target in the
// platform's default browser.
func browserOpenArgs(goos, target string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{target}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}, nil
	case "linux":
		return "xdg-open", []string{target}, nil
	default:
		return "", nil, fmt.Errorf("unsupported platform %q; open %s manually", goos, target)
	}
}
