package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmontp19/bonpreu-cli/internal/api"
)

func newDeliveryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "delivery",
		Short:         "Delivery destinations (addresses / pickup points)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newDeliveryAddressesCmd(), newDeliveryUseCmd())
	return cmd
}

func newDeliveryAddressesCmd() *cobra.Command {
	var method string
	var postal string
	cmd := &cobra.Command{
		Use:           "addresses",
		Short:         "List delivery addresses (--method home) or pickup points (--method cc)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			m, _, err := api.GroupParams(method)
			if err != nil {
				return err
			}
			addrs, err := api.GetDeliveryAddresses(ctx, rt.client, m)
			if err != nil {
				return err
			}
			addrs = filterByPostal(addrs, postal)
			if rt.json {
				return printJSON(addrs)
			}
			if len(addrs) == 0 {
				if postal != "" {
					fmt.Fprintf(os.Stderr, "no delivery destinations matching postal code %q\n", postal)
				} else {
					fmt.Fprintln(os.Stderr, "no delivery destinations")
				}
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "DESTID\tPRIMARY\tPOSTAL\tNAME\tADDRESS")
			for _, a := range addrs {
				primary := ""
				if a.IsPrimary {
					primary = "*"
				}
				// Full DESTID (not short()): `slots --destination` needs the
				// exact UUID.
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", a.DeliveryDestinationID, primary, a.PostalCode, a.Name, a.FormattedAddress)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVarP(&method, "method", "m", "home", "delivery method: home|cc")
	cmd.Flags().StringVar(&postal, "postal", "", "filter by postal code prefix (e.g. 08800 or 088)")
	return cmd
}

// filterByPostal keeps only addresses whose postal code starts with prefix;
// an empty prefix returns addrs unchanged.
func filterByPostal(addrs []api.DeliveryAddress, prefix string) []api.DeliveryAddress {
	if prefix == "" {
		return addrs
	}
	out := make([]api.DeliveryAddress, 0, len(addrs))
	for _, a := range addrs {
		if strings.HasPrefix(a.PostalCode, prefix) {
			out = append(out, a)
		}
	}
	return out
}

func newDeliveryUseCmd() *cobra.Command {
	var group string
	cmd := &cobra.Command{
		Use:           "use <destinationId>",
		Short:         "Remember a destination as the default for a group (home|cc), like the website remembers your last pick",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			m, _, err := api.GroupParams(group)
			if err != nil {
				return err
			}
			addrs, err := api.GetDeliveryAddresses(ctx, rt.client, m)
			if err != nil {
				return err
			}
			found := false
			for _, a := range addrs {
				if a.DeliveryDestinationID == args[0] {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("destination %q not found among %s addresses; run `delivery addresses --method %s`", args[0], group, group)
			}
			cfg, err := loadConfig(rt.flags)
			if err != nil {
				return err
			}
			if cfg.DefaultDestinations == nil {
				cfg.DefaultDestinations = map[string]string{}
			}
			cfg.DefaultDestinations[group] = args[0]
			if err := saveConfig(rt.flags, cfg); err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "default %s destination set to %s\n", group, args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&group, "group", "g", "home", "which group's default to set: home|cc")
	return cmd
}

func newSlotsCmd() *cobra.Command {
	var group string
	var destination string
	var days int
	cmd := &cobra.Command{
		Use:           "slots",
		Short:         "List available delivery slots (--group home|cc)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			dest, err := resolveDefaultDestination(rt.flags, group, destination)
			if err != nil {
				return err
			}
			res, err := api.GetSlots(ctx, rt.client, group, dest, days)
			if err != nil {
				return err
			}
			if rt.json {
				return printJSON(res)
			}
			if len(res.Slots) == 0 {
				fmt.Fprintln(os.Stderr, "no slots available")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SLOTID\tDAY\tWINDOW\tPRICE\tAVAILABLE")
			for _, s := range res.Slots {
				avail := "no"
				if s.Available {
					avail = "yes"
				}
				window := fmt.Sprintf("%s–%s", clockTime(s.StartTime), clockTime(s.EndTime))
				// Full slot ID (not short()): `slots reserve` needs the exact
				// UUID and there is no slot-id resolver.
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.SlotID, s.Day, window, s.Price, avail)
			}
			_ = w.Flush()
			if res.MinimumOrderValue != nil && res.MinimumOrderValue.Amount != "" {
				fmt.Fprintf(os.Stdout, "minimum order: %s €\n", res.MinimumOrderValue.Amount)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&group, "group", "g", "home", "slot group: home|cc")
	cmd.Flags().StringVar(&destination, "destination", "", "delivery destination id to use instead of the default (see `delivery addresses`)")
	cmd.Flags().IntVarP(&days, "days", "d", 7, "number of days to fetch")
	cmd.AddCommand(newSlotsReserveCmd())
	return cmd
}

func newSlotsReserveCmd() *cobra.Command {
	var group string
	var destination string
	cmd := &cobra.Command{
		Use:           "reserve <slotId>",
		Short:         "Reserve a delivery slot for checkout",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			dest, err := resolveDefaultDestination(rt.flags, group, destination)
			if err != nil {
				return err
			}
			res, err := api.ReserveSlot(ctx, rt.client, group, dest, args[0])
			if err != nil {
				return err
			}
			if rt.json {
				return printJSON(res)
			}
			fmt.Fprintf(os.Stdout, "reserved %s\n", res.Slot.SlotID)
			fmt.Fprintf(os.Stdout, "window:  %s–%s\n", res.Slot.SlotWindow.StartTime, res.Slot.SlotWindow.EndTime)
			fmt.Fprintf(os.Stdout, "method:  %s\n", res.Slot.DeliveryMethod)
			if res.Slot.ExpiryTime != "" {
				fmt.Fprintf(os.Stdout, "expires: %s\n", res.Slot.ExpiryTime)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&group, "group", "g", "home", "slot group the slot was fetched with: home|cc")
	cmd.Flags().StringVar(&destination, "destination", "", "delivery destination id the slot was fetched with (must match `slots --destination`)")
	return cmd
}

// resolveDefaultDestination returns the explicit --destination override if
// set, else the group's remembered default from config.json (set via
// `delivery use`), else "" to let the API pick automatically.
func resolveDefaultDestination(f *Flags, group, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	cfg, err := loadConfig(f)
	if err != nil {
		return "", err
	}
	return cfg.DefaultDestinations[group], nil
}

// clockTime formats an RFC3339 slot timestamp as a bare HH:MM for the
// slots table; the date is already shown in the DAY column.
func clockTime(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	return t.Format("15:04")
}
