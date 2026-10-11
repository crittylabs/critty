package cli

import (
	"fmt"

	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/review"
	"github.com/spf13/cobra"
)

var stopSession string

var stopCmd = &cobra.Command{
	Use:   "stop --session <id>",
	Short: "Stop a review TUI and keep its saved state",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, args []string) error { return stopReview(stopSession) },
}

func stopReview(key string) error {
	if !review.ValidSessionKey(key) {
		return fmt.Errorf("a valid --session ID is required")
	}
	if _, err := review.ReadSessionEntry(key); err != nil {
		return err
	}
	sock := review.SocketPathFor(key)
	if ipc.Alive(sock) {
		if err := ipc.Stop(sock); err != nil {
			return err
		}
	}
	fmt.Printf("Stopped review %s; saved state retained\n", key)
	return nil
}

func init() {
	rootCmd.AddCommand(stopCmd)
	stopCmd.Flags().StringVar(&stopSession, "session", "", "review session to stop")
}
