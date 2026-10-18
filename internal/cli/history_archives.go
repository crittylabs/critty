package cli

import "github.com/spf13/cobra"

func newArchivesCommand() *cobra.Command {
	command := &cobra.Command{Use: "archives", Short: "List, verify, or explicitly delete saved archives"}
	var directory string
	list := &cobra.Command{Use: "list", Short: "List archives for this working directory, including custom exports", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, policy, err := historyContext()
			if err != nil {
				return err
			}
			root := policy.ArchiveDir
			if cmd.Flags().Changed("directory") {
				root = directory
			}
			result, err := store.Archives(root)
			return historyOutput(cmd, result, err)
		},
	}
	list.Flags().StringVar(&directory, "directory", "", "also inspect this parent directory for copied archives")
	verify := &cobra.Command{Use: "verify <archive-path>", Short: "Verify a selected archive's recorded files", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, err := historyContext()
			if err != nil {
				return err
			}
			return historyOutput(cmd, map[string]string{"archive": args[0]}, store.VerifyArchive(args[0]))
		},
	}
	remove := &cobra.Command{Use: "delete <archive-path>", Short: "Delete only the selected verified archive; keep original reviews", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, err := historyContext()
			if err != nil {
				return err
			}
			return historyOutput(cmd, map[string]string{"archive": args[0]}, store.DeleteArchive(args[0]))
		},
	}
	command.AddCommand(list, verify, remove)
	return command
}
