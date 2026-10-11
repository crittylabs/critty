package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/crittylabs/critty/internal/terminalbinding"
	"github.com/spf13/cobra"
)

var terminalRequest string

func init() {
	terminal := &cobra.Command{Use: "terminal", Short: "Locate a review terminal using a one-time visible marker"}
	terminal.AddCommand(&cobra.Command{
		Use: "prepare", Short: "Create a two-minute terminal request and print its JSON marker", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := terminalbinding.DefaultStore().Prepare()
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(r)
		},
	})
	terminal.AddCommand(&cobra.Command{
		Use: "notify <request-id>", Short: "Report a detected marker from a terminal filter's own environment", Args: cobra.ExactArgs(1),
		Long: "Called by a terminal filter such as tfil after observing CRITTY-<request-id> in rendered agent output. Inherit the filter's original TMUX/TMUX_PANE or HERDR_* environment and the same XDG_STATE_HOME as Critty. This command records a destination; it does not open a review.",
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := terminalbinding.FromEnvironment()
			if err != nil {
				return err
			}
			if target.Kind == "" {
				return fmt.Errorf("notification requires the filter's tmux or Herdr environment")
			}
			return terminalbinding.DefaultStore().Notify(args[0], target)
		},
	})
	rootCmd.AddCommand(terminal)
	for _, cmd := range []*cobra.Command{rootCmd, reviewCmd, planCmd, envCmd} {
		cmd.Flags().StringVar(&terminalRequest, "terminal-request", "", "locate this review using a prepared terminal marker request")
	}
}

func reviewTerminalContext() (reviewMultiplexerContext, error) {
	if terminalRequest == "" {
		return findMultiplexerContext(), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	target, err := terminalbinding.DefaultStore().Resolve(ctx, terminalRequest, terminalbinding.Snapshot)
	if err != nil {
		return nil, err
	}
	if target.Kind == "tmux" {
		return tmuxContext{socket: target.Socket, pane: target.Pane}, nil
	}
	return herdrContext{socket: target.Socket, workspace: target.Workspace, tab: target.Tab, pane: target.Pane}, nil
}
