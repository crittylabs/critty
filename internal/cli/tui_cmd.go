package cli

import (
	"github.com/crittylabs/critty/internal/config"
	"github.com/spf13/cobra"
)

var tuiSession string

var tuiCmd = &cobra.Command{
	Use:    "_tui --session <id>",
	Short:  "Run one internal review round",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadCurrent()
		if err != nil {
			return err
		}
		sess, mode, err := loadReviewSession(tuiSession)
		if err != nil {
			return err
		}
		if err := resolveFocus(mode); err != nil {
			return err
		}
		_, err = runTUISession(cfg, sess, mode, true)
		return err
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
	addFocusFlag(tuiCmd)
	tuiCmd.Flags().StringVar(&tuiSession, "session", "", "saved review session")
}
