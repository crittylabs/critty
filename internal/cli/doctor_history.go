package cli

import (
	"fmt"
	"strings"
)

func (r *doctorReport) checkHistory() {
	store, policy, err := historyContext()
	if err != nil {
		r.add("history/policy", "error", err.Error(), "Check the global history settings and working directory before maintenance.")
		return
	}
	report := store.Inspect(policy)
	for _, check := range []struct{ kind, healthy, advice string }{
		{"capacity", fmt.Sprintf("known original reviews use %d of %d allocated bytes", report.Storage.Bytes, report.Storage.MaxBytes), "Use `critty history usage` for measured roots. `critty history clean` applies start policy; auto rotation protects archives and ongoing or busy work."},
		{"age", "no idle tasks or ungrouped reviews beyond keep_days", "Inspect `critty history list` and `critty status`. Use the critty-history skill to choose continuation, saving, or cleanup; age alone does not authorize removal."},
		{"recovery", "no interrupted task cleanup", "Inspect `critty history list`; resume only the recorded task/action with `critty history finish <task-id> --action <recorded-action>`."},
		{"incomplete", "known history locations inspected", "Use `critty history list`, `critty status`, and reported CLI recovery commands. Retain unavailable records and report blockers the CLI cannot resolve; do not edit metadata by hand. Archives and external copies are outside the backup estimate."},
	} {
		status, message := "ok", check.healthy
		if issue := report.Issues[check.kind]; issue != "" {
			status, message = "warn", issue
		}
		advice := ""
		if status == "warn" || check.kind == "capacity" {
			advice = check.advice
		}
		if check.kind == "incomplete" && len(report.Storage.Warnings) != 0 {
			advice += " " + strings.Join(report.Storage.Warnings, "; ")
		}
		r.add("history/"+check.kind, status, message, advice)
	}
}
