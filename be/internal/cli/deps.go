package cli

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"be/internal/client"
	"be/internal/model"
)

func init() {
	rootCmd.AddCommand(depsCmd)
}

var depsCmd = &cobra.Command{
	Use:   "deps",
	Short: "Dependency management commands",
}

// --- list ---

func init() {
	listCmd := &cobra.Command{
		Use:   "list <ticket-id>",
		Short: "List dependencies for a ticket",
		Args:  cobra.ExactArgs(1),
		RunE:  runDepsList,
	}
	depsCmd.AddCommand(listCmd)
}

func runDepsList(cmd *cobra.Command, args []string) error {
	if err := RequireProject(); err != nil {
		return err
	}
	c := getHTTPClient()

	var resp struct {
		Blockers []*model.Dependency `json:"blockers"`
		Blocks   []*model.Dependency `json:"blocks"`
	}
	if err := c.Get("/api/v1/tickets/"+url.PathEscape(args[0])+"/dependencies", &resp); err != nil {
		return err
	}

	if ticketsJSON {
		out, err := client.FormatJSON(resp)
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	}

	fmt.Printf("Dependencies for %s:\n\n", args[0])

	if len(resp.Blockers) > 0 {
		fmt.Println("Blocked by:")
		for _, d := range resp.Blockers {
			fmt.Printf("  %s\n", d.DependsOnID)
		}
	} else {
		fmt.Println("Blocked by: (none)")
	}

	if len(resp.Blocks) > 0 {
		fmt.Println("\nBlocks:")
		for _, d := range resp.Blocks {
			fmt.Printf("  %s\n", d.IssueID)
		}
	} else {
		fmt.Println("\nBlocks: (none)")
	}

	return nil
}

// --- add ---

func init() {
	addCmd := &cobra.Command{
		Use:   "add <ticket-id> <blocker-id>",
		Short: "Add a dependency (ticket-id is blocked by blocker-id)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireProject(); err != nil {
				return err
			}
			c := getHTTPClient()

			body := map[string]string{
				"issue_id":      args[0],
				"depends_on_id": args[1],
			}

			var resp struct {
				Message string `json:"message"`
			}
			if err := c.Post("/api/v1/dependencies", body, &resp); err != nil {
				return err
			}

			fmt.Printf("Added: %s is blocked by %s\n", args[0], args[1])
			return nil
		},
	}
	depsCmd.AddCommand(addCmd)
}

// --- remove ---

func init() {
	removeCmd := &cobra.Command{
		Use:   "remove <ticket-id> <blocker-id>",
		Short: "Remove a dependency",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireProject(); err != nil {
				return err
			}
			c := getHTTPClient()

			body := map[string]string{
				"issue_id":      args[0],
				"depends_on_id": args[1],
			}

			var resp struct {
				Message string `json:"message"`
			}
			if err := c.Delete("/api/v1/dependencies", body, &resp); err != nil {
				return err
			}

			fmt.Printf("Removed: %s no longer blocked by %s\n", args[0], args[1])
			return nil
		},
	}
	depsCmd.AddCommand(removeCmd)
}
