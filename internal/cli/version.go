package cli

import (
	"io"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/tui"
)

// version is set at release build time via -ldflags "-X ...internal/cli.version=...".
var version = ""

func versionString() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(unknown)"
}

// writeVersion prints the plain Cobra version line, or the Help dialog's
// brand and version when w is a color-capable terminal.
func writeVersion(w io.Writer, name, version string) error {
	style := detectOutputStyle(w)
	if !style.color() {
		_, err := io.WriteString(w, name+" version "+version+"\n")
		return err
	}
	_, err := io.WriteString(style.writer(w), tui.BrandVersion(version, style.dark)+"\n")
	return err
}

func init() {
	cobra.AddTemplateFunc("crittyVersion", func(c *cobra.Command) string {
		if err := writeVersion(c.OutOrStdout(), c.DisplayName(), c.Version); err != nil {
			c.PrintErrln(err)
		}
		return ""
	})
	rootCmd.SetVersionTemplate("{{crittyVersion .}}")
}
