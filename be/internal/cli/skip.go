package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var skipCmd = &cobra.Command{
	Use:   "skip <tag>",
	Short: "Add a skip tag to the current workflow instance",
	Long: `Add a skip tag to the running workflow instance. The tag must be one of the
workflow's defined groups. The instance is identified by NRWF_WORKFLOW_INSTANCE_ID
env var (set automatically by the spawner).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := CheckServer(); err != nil {
			return err
		}

		instanceID := GetWorkflowInstanceID()
		if instanceID == "" {
			return fmt.Errorf("NRWF_WORKFLOW_INSTANCE_ID is required (set by spawner)")
		}

		tag := args[0]

		c := GetClient()
		reqParams := map[string]interface{}{
			"instance_id": instanceID,
			"tag":         tag,
		}

		if err := c.ExecuteAndUnmarshal("workflow.skip", reqParams, nil); err != nil {
			return err
		}

		fmt.Printf("Skip tag '%s' added to workflow instance\n", tag)
		return nil
	},
}
