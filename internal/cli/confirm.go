package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// confirmPrompt asks a y/N question, reading one line from the command's input
// stream (so tests can drive it via cmd.SetIn). It returns true only on an
// explicit "y"/"yes". A non-interactive stream that reaches EOF with no input
// reads as "no", so scripts must pass an explicit bypass flag rather than rely
// on a default.
func confirmPrompt(cmd *cobra.Command, prompt string) (bool, error) {
	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N]: ", prompt)
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
