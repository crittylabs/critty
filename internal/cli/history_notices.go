package cli

import (
	"fmt"
	"io"

	"github.com/crittylabs/critty/internal/history"
)

func warnHistory(w io.Writer, cwd string, policy history.Policy) {
	messages, err := history.New(cwd).Notices(policy)
	for _, message := range messages {
		fmt.Fprintln(w, "critty: warning: history:", message)
	}
	if err != nil {
		fmt.Fprintln(w, "critty: warning: history: check unavailable:", err)
	}
}
