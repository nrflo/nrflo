package cli

import (
	"github.com/spf13/cobra"

	nrvappscaffold "be/internal/nrvapp/scaffold"
)

var initCustomerCmd = &cobra.Command{
	Use:   "init-customer",
	Short: "Scaffold a starter customer config directory for nrvapp tools (used by api-mode workflows)",
	Long: `Scaffold a new customer config directory containing a tool_manifest.yaml
and example Python scripts. Use --out to specify the target directory.

Flags are passed through to the scaffold runner:
  --out    <dir>   output directory (required)
  --name   <name>  customer name for template substitution (default: basename of --out)
  --force          overwrite non-empty target directory
  --git            initialize a git repository after scaffolding`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nrvappscaffold.Run(args)
	},
}
