package cmds

import "github.com/spf13/cobra"

func init() {
	RootCmd.AddCommand(reverseCmd)
}

var reverseCmd = &cobra.Command{
	Use:   "reverse",
	Short: "reverse",
	Long:  `reverse`,
}
