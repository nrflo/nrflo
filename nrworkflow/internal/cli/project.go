package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"nrworkflow/internal/client"
	"nrworkflow/internal/model"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
	Long:  `Create, list, show, and delete projects.`,
}

// Project create flags
var (
	projectCreateName            string
	projectCreateRootPath        string
	projectCreateDefaultWorkflow string
)

var projectCreateCmd = &cobra.Command{
	Use:   "create <project-id>",
	Short: "Create a new project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := CheckServer(); err != nil {
			return err
		}

		projectID := strings.ToLower(args[0])

		c := GetClient()
		params := map[string]interface{}{
			"id": projectID,
		}
		if projectCreateName != "" {
			params["name"] = projectCreateName
		}
		if projectCreateRootPath != "" {
			params["root_path"] = projectCreateRootPath
		}
		if projectCreateDefaultWorkflow != "" {
			params["default_workflow"] = projectCreateDefaultWorkflow
		}

		var project model.Project
		if err := c.ExecuteAndUnmarshal("project.create", params, &project); err != nil {
			return fmt.Errorf("failed to create project: %w", err)
		}

		fmt.Printf("Created project: %s\n", projectID)
		return nil
	},
}

var projectListJSON bool

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := CheckServer(); err != nil {
			return err
		}

		c := GetClient()

		var projects []*model.Project
		if err := c.ExecuteAndUnmarshal("project.list", nil, &projects); err != nil {
			return fmt.Errorf("failed to list projects: %w", err)
		}

		output, err := client.FormatProjectList(projects, projectListJSON)
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

var projectShowJSON bool

var projectShowCmd = &cobra.Command{
	Use:   "show <project-id>",
	Short: "Show project details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := CheckServer(); err != nil {
			return err
		}

		projectID := args[0]
		c := GetClient()

		params := map[string]string{"id": projectID}

		var project model.Project
		if err := c.ExecuteAndUnmarshal("project.get", params, &project); err != nil {
			return err
		}

		output, err := client.FormatProjectShow(&project, projectShowJSON)
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

var projectDeleteForce bool

var projectDeleteCmd = &cobra.Command{
	Use:   "delete <project-id>",
	Short: "Delete a project",
	Long:  `Delete a project and all its tickets.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := CheckServer(); err != nil {
			return err
		}

		projectID := args[0]

		if !projectDeleteForce {
			fmt.Printf("This will delete project '%s' and ALL its tickets.\n", projectID)
			fmt.Print("Are you sure? (y/N): ")
			var response string
			fmt.Scanln(&response)
			if strings.ToLower(response) != "y" {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		c := GetClient()
		params := map[string]string{"id": projectID}

		if err := c.ExecuteAndUnmarshal("project.delete", params, nil); err != nil {
			return fmt.Errorf("failed to delete project: %w", err)
		}

		fmt.Printf("Deleted project: %s\n", projectID)
		return nil
	},
}

func init() {
	// project create
	projectCreateCmd.Flags().StringVar(&projectCreateName, "name", "", "Project display name")
	projectCreateCmd.Flags().StringVar(&projectCreateRootPath, "root", "", "Project root path")
	projectCreateCmd.Flags().StringVar(&projectCreateDefaultWorkflow, "workflow", "", "Default workflow")
	projectCmd.AddCommand(projectCreateCmd)

	// project list
	projectListCmd.Flags().BoolVar(&projectListJSON, "json", false, "Output in JSON format")
	projectCmd.AddCommand(projectListCmd)

	// project show
	projectShowCmd.Flags().BoolVar(&projectShowJSON, "json", false, "Output in JSON format")
	projectCmd.AddCommand(projectShowCmd)

	// project delete
	projectDeleteCmd.Flags().BoolVar(&projectDeleteForce, "force", false, "Skip confirmation")
	projectCmd.AddCommand(projectDeleteCmd)
}
