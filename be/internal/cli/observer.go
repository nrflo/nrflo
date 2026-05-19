package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var observerCmd = &cobra.Command{
	Use:           "observer",
	Short:         "Observer commands (env-gated)",
	Hidden:        true,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func unknownCmdErr(name, root string) error {
	return fmt.Errorf("unknown command %q for %q", name, root)
}

// runObserverMethod dispatches an observer socket method and prints the JSON result.
// RequireProject is enforced for workflow and project scopes; not required for global.
func runObserverMethod(method string, params map[string]interface{}) error {
	scope := os.Getenv("NRF_OBSERVER_SCOPE")
	if scope == "workflow" || scope == "project" {
		if err := RequireProject(); err != nil {
			return err
		}
	}
	if err := CheckServer(); err != nil {
		return err
	}
	params["session_id"] = GetSessionID()
	var result json.RawMessage
	if err := GetClient().ExecuteAndUnmarshal(method, params, &result); err != nil {
		return err
	}
	if len(result) > 0 {
		fmt.Println(string(result))
	}
	return nil
}

func init() {
	observerCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if os.Getenv("NRF_OBSERVER") != "1" {
			return unknownCmdErr("observer", cmd.Root().Name())
		}
		observerCmd.Hidden = false
		scope := os.Getenv("NRF_OBSERVER_SCOPE")
		for _, child := range observerCmd.Commands() {
			child.Hidden = child.Use != scope
		}
		return nil
	}
	observerCmd.AddCommand(workflowGroupCmd, projectGroupCmd, globalGroupCmd)
}
