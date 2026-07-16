package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// confirmPrompt asks a y/N question, reading one line from the command's input
// stream (so tests can drive it via cmd.SetIn). It returns true only on an
// explicit "y"/"yes". Any read problem — EOF with no input on a non-interactive
// stream, or a genuine I/O error — is treated as "no", so a destructive command
// never proceeds on ambiguity; scripts must pass an explicit bypass flag.
func confirmPrompt(cmd *cobra.Command, prompt string) bool {
	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N]: ", prompt)
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
