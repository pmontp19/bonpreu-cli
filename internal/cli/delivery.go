package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/pmontp19/bonpreu-cli/internal/api"
)

// groupMethod maps the user-facing --method/--group value to the API delivery
// method constant.
func groupMethod(v string) (string, error) {
	switch v {
	case "home", "":
		return api.MethodHome, nil
	case "cc":
		return api.MethodCC, nil
	default:
		return "", fmt.Errorf("unknown value %q (want home|cc)", v)
	}
}

func newDeliveryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "delivery",
		Short:         "Delivery destinations (addresses / pickup points)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newDeliveryAddressesCmd())
	return cmd
}

func newDeliveryAddressesCmd() *cobra.Command {
	var method string
	cmd := &cobra.Command{
		Use:           "addresses",
		Short:         "List delivery addresses (--method home) or pickup points (--method cc)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			m, err := groupMethod(method)
			if err != nil {
				return err
			}
			addrs, err := api.GetDeliveryAddresses(ctx, rt.client, m)
			if err != nil {
				return err
			}
			if rt.json {
				return printJSON(addrs)
			}
			if len(addrs) == 0 {
				fmt.Fprintln(os.Stderr, "no delivery destinations")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "DESTID\tPRIMARY\tPOSTAL\tNAME\tADDRESS")
			for _, a := range addrs {
				primary := ""
				if a.IsPrimary {
					primary = "*"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", short(a.DeliveryDestinationID), primary, a.PostalCode, a.Name, a.FormattedAddress)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVarP(&method, "method", "m", "home", "delivery method: home|cc")
	return cmd
}

func newSlotsCmd() *cobra.Command {
	var group string
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
			res, err := api.GetSlots(ctx, rt.client, group, days)
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
				window := fmt.Sprintf("%s–%s", s.StartTime, s.EndTime)
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", short(s.SlotID), s.Day, window, s.Price, avail)
			}
			_ = w.Flush()
			if res.MinimumOrderValue != nil && res.MinimumOrderValue.Amount != "" {
				fmt.Fprintf(os.Stdout, "minimum order: %s €\n", res.MinimumOrderValue.Amount)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&group, "group", "g", "home", "slot group: home|cc")
	cmd.Flags().IntVarP(&days, "days", "d", 7, "number of days to fetch")
	cmd.AddCommand(newSlotsReserveCmd())
	return cmd
}

func newSlotsReserveCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "reserve <slotId>",
		Short:         "Reserve a delivery slot for checkout",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rt := ctxValue(ctx)
			res, err := api.ReserveSlot(ctx, rt.client, args[0])
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
}
