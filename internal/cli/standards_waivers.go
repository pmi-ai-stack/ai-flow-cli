package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
)

// waiverExpiringDays is how far ahead a waiver counts as "expiring":
// the default window of `standards waivers list` and the one `doctor`
// warns on.
const waiverExpiringDays = 14

type waiverListEntry struct {
	CheckID  string `json:"check_id"`
	Reason   string `json:"reason"`
	Author   string `json:"author"`
	Created  string `json:"created"`
	Expires  string `json:"expires"`
	State    string `json:"state"`
	DaysLeft int    `json:"days_left"`
}

type standardsWaiversListResult struct {
	Dir        string            `json:"dir"`
	WindowDays int               `json:"window_days"`
	Active     int               `json:"active"`
	Expiring   int               `json:"expiring"`
	Expired    int               `json:"expired"`
	Waivers    []waiverListEntry `json:"waivers"`
}

func newStandardsWaiversCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "waivers",
		Short: "Inspect a repository's waivers",
	}
	cmd.AddCommand(newStandardsWaiversListCmd(cio))
	return cmd
}

func newStandardsWaiversListCmd(cio cmdIO) *cobra.Command {
	var expiring int
	cmd := &cobra.Command{
		Use:   "list DIR",
		Short: "List DIR's waivers with their expiry state; --expiring DAYS shows only those expired or lapsing within DAYS",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("standards waivers list", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			dir := args[0]
			if expiring < 0 {
				return Outcome{}, &envelope.Error{Code: "usage", Message: fmt.Sprintf("--expiring %d must be zero or more days", expiring)}
			}
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				return Outcome{}, &envelope.Error{Code: "usage", Message: fmt.Sprintf("%s is not a directory", dir)}
			}
			window, onlyLapsing := waiverExpiringDays, cmd.Flags().Changed("expiring")
			if onlyLapsing {
				window = expiring
			}
			res, err := listWaivers(dir, handoff.Now(), window, onlyLapsing)
			if err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: res}, nil
		}),
	}
	cmd.Flags().IntVar(&expiring, "expiring", 0, "show only waivers already expired or expiring within this many days")
	return cmd
}

// listWaivers reads dir's waivers and classifies each at now. Counts
// always cover every waiver in the file; onlyLapsing trims the list,
// not the counts.
func listWaivers(dir string, now time.Time, windowDays int, onlyLapsing bool) (standardsWaiversListResult, error) {
	ws, err := standards.LoadWaivers("", dir)
	if err != nil {
		return standardsWaiversListResult{}, err
	}
	res := standardsWaiversListResult{Dir: dir, WindowDays: windowDays, Waivers: []waiverListEntry{}}
	for _, w := range ws {
		state, days := standards.ExpiryState(w, now, time.Duration(windowDays)*24*time.Hour)
		switch state {
		case standards.StateExpired:
			res.Expired++
		case standards.StateExpiring:
			res.Expiring++
		default:
			res.Active++
		}
		if onlyLapsing && state == standards.StateActive {
			continue
		}
		res.Waivers = append(res.Waivers, waiverListEntry{
			CheckID: w.CheckID, Reason: w.Reason, Author: w.Author, Created: w.Created, Expires: w.Expires,
			State: state, DaysLeft: days,
		})
	}
	return res, nil
}
