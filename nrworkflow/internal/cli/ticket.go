package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"nrworkflow/internal/client"
	"nrworkflow/internal/model"
	"nrworkflow/internal/types"
)

var ticketCmd = &cobra.Command{
	Use:   "ticket",
	Short: "Manage tickets",
	Long:  `Create, list, show, update, close, and delete tickets.`,
}

// RequireProject is a helper that ensures ProjectID is set
func RequireProject() error {
	if ProjectID == "" {
		return fmt.Errorf("project not found. Create .claude/nrworkflow/config.json with 'project' field, or set NRWORKFLOW_PROJECT env")
	}
	return nil
}

// Ticket create flags
var (
	ticketCreateTitle       string
	ticketCreateType        string
	ticketCreatePriority    int
	ticketCreateDescription string
	ticketCreateID          string
)

var ticketCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new ticket",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		if ticketCreateTitle == "" {
			return fmt.Errorf("--title is required")
		}

		c := GetClient()
		params := types.TicketCreateRequest{
			ID:          ticketCreateID,
			Title:       ticketCreateTitle,
			Type:        ticketCreateType,
			Priority:    ticketCreatePriority,
			Description: ticketCreateDescription,
		}

		var ticket model.Ticket
		if err := c.ExecuteAndUnmarshal("ticket.create", params, &ticket); err != nil {
			return fmt.Errorf("failed to create ticket: %w", err)
		}

		fmt.Println(ticket.ID)
		return nil
	},
}

// Ticket list flags
var (
	ticketListStatus string
	ticketListType   string
	ticketListJSON   bool
)

var ticketListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tickets",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		c := GetClient()
		params := types.TicketListRequest{
			Status: ticketListStatus,
			Type:   ticketListType,
		}

		var tickets []*model.Ticket
		if err := c.ExecuteAndUnmarshal("ticket.list", params, &tickets); err != nil {
			return fmt.Errorf("failed to list tickets: %w", err)
		}

		output, err := client.FormatTicketList(tickets, ticketListJSON)
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

// Ticket show flags
var ticketShowJSON bool

var ticketShowCmd = &cobra.Command{
	Use:   "show <ticket-id>",
	Short: "Show ticket details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		c := GetClient()

		params := map[string]string{"id": ticketID}

		var ticket model.Ticket
		if err := c.ExecuteAndUnmarshal("ticket.get", params, &ticket); err != nil {
			return err
		}

		output, err := client.FormatTicketShow(&ticket, ticketShowJSON)
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

// Ticket update flags
var (
	ticketUpdateTitle       string
	ticketUpdateDescription string
	ticketUpdateStatus      string
	ticketUpdatePriority    int
	ticketUpdateType        string
	ticketUpdateAgentsState string
)

var ticketUpdateCmd = &cobra.Command{
	Use:   "update <ticket-id>",
	Short: "Update a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		c := GetClient()

		params := map[string]interface{}{"id": ticketID}

		if cmd.Flags().Changed("title") {
			params["title"] = ticketUpdateTitle
		}
		if cmd.Flags().Changed("d") {
			params["description"] = ticketUpdateDescription
		}
		if cmd.Flags().Changed("status") {
			params["status"] = ticketUpdateStatus
		}
		if cmd.Flags().Changed("priority") {
			params["priority"] = ticketUpdatePriority
		}
		if cmd.Flags().Changed("type") {
			params["type"] = ticketUpdateType
		}
		if cmd.Flags().Changed("agents_state") {
			params["agents_state"] = ticketUpdateAgentsState
		}

		if err := c.ExecuteAndUnmarshal("ticket.update", params, nil); err != nil {
			return fmt.Errorf("failed to update ticket: %w", err)
		}

		fmt.Printf("Updated: %s\n", ticketID)
		return nil
	},
}

// Ticket close flags
var ticketCloseReason string

var ticketCloseCmd = &cobra.Command{
	Use:   "close <ticket-id>",
	Short: "Close a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]
		c := GetClient()

		params := map[string]string{
			"id":     ticketID,
			"reason": ticketCloseReason,
		}

		if err := c.ExecuteAndUnmarshal("ticket.close", params, nil); err != nil {
			return fmt.Errorf("failed to close ticket: %w", err)
		}

		fmt.Printf("Closed: %s\n", ticketID)
		return nil
	},
}

// Ticket delete flags
var ticketDeleteForce bool

var ticketDeleteCmd = &cobra.Command{
	Use:   "delete <ticket-id>",
	Short: "Delete a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		ticketID := args[0]

		if !ticketDeleteForce {
			fmt.Printf("Delete ticket '%s'? (y/N): ", ticketID)
			var response string
			fmt.Scanln(&response)
			if strings.ToLower(response) != "y" {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		c := GetClient()
		params := map[string]string{"id": ticketID}

		if err := c.ExecuteAndUnmarshal("ticket.delete", params, nil); err != nil {
			return fmt.Errorf("failed to delete ticket: %w", err)
		}

		fmt.Printf("Deleted: %s\n", ticketID)
		return nil
	},
}

// Ticket search
var ticketSearchJSON bool

var ticketSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search tickets",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		query := args[0]
		c := GetClient()

		params := types.TicketSearchRequest{Query: query}

		var tickets []*model.Ticket
		if err := c.ExecuteAndUnmarshal("ticket.search", params, &tickets); err != nil {
			return fmt.Errorf("failed to search tickets: %w", err)
		}

		if ticketSearchJSON {
			output, _ := json.MarshalIndent(tickets, "", "  ")
			fmt.Println(string(output))
			return nil
		}

		if len(tickets) == 0 {
			fmt.Println("No tickets found.")
			return nil
		}

		fmt.Printf("%-15s %-12s %-10s %s\n", "ID", "TYPE", "STATUS", "TITLE")
		fmt.Println("---------------------------------------------------------------")
		for _, t := range tickets {
			title := t.Title
			if len(title) > 40 {
				title = title[:37] + "..."
			}
			fmt.Printf("%-15s %-12s %-10s %s\n", t.ID, t.IssueType, t.Status, title)
		}

		return nil
	},
}

// Ticket dep command group
var ticketDepCmd = &cobra.Command{
	Use:   "dep",
	Short: "Manage ticket dependencies",
}

var ticketDepAddCmd = &cobra.Command{
	Use:   "add <child> <parent>",
	Short: "Add a dependency (child depends on parent)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		child := args[0]
		parent := args[1]
		c := GetClient()

		params := types.DependencyRequest{
			Child:  child,
			Parent: parent,
		}

		if err := c.ExecuteAndUnmarshal("ticket.dep.add", params, nil); err != nil {
			return fmt.Errorf("failed to add dependency: %w", err)
		}

		fmt.Printf("Added: %s depends on %s\n", child, parent)
		return nil
	},
}

var ticketDepRemoveCmd = &cobra.Command{
	Use:   "remove <child> <parent>",
	Short: "Remove a dependency",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		child := args[0]
		parent := args[1]
		c := GetClient()

		params := types.DependencyRequest{
			Child:  child,
			Parent: parent,
		}

		if err := c.ExecuteAndUnmarshal("ticket.dep.remove", params, nil); err != nil {
			return fmt.Errorf("failed to remove dependency: %w", err)
		}

		fmt.Printf("Removed: %s no longer depends on %s\n", child, parent)
		return nil
	},
}

// Ticket ready command
var ticketReadyJSON bool

var ticketReadyCmd = &cobra.Command{
	Use:   "ready",
	Short: "List tickets that are ready to work on",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		c := GetClient()

		var tickets []*model.Ticket
		if err := c.ExecuteAndUnmarshal("ticket.ready", nil, &tickets); err != nil {
			return fmt.Errorf("failed to get ready tickets: %w", err)
		}

		if ticketReadyJSON {
			output, _ := json.MarshalIndent(tickets, "", "  ")
			fmt.Println(string(output))
			return nil
		}

		if len(tickets) == 0 {
			fmt.Println("No ready tickets found.")
			return nil
		}

		fmt.Printf("%-15s %-12s %-8s %s\n", "ID", "TYPE", "PRIORITY", "TITLE")
		fmt.Println("---------------------------------------------------------------")
		for _, t := range tickets {
			title := t.Title
			if len(title) > 40 {
				title = title[:37] + "..."
			}
			fmt.Printf("%-15s %-12s %-8d %s\n", t.ID, t.IssueType, t.Priority, title)
		}

		return nil
	},
}

// Ticket status command
var (
	ticketStatusPending   int
	ticketStatusCompleted int
	ticketStatusJSON      bool
)

var ticketStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show ticket status summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireProject(); err != nil {
			return err
		}
		if err := CheckServer(); err != nil {
			return err
		}

		c := GetClient()

		params := types.StatusRequest{
			PendingLimit:   ticketStatusPending,
			CompletedLimit: ticketStatusCompleted,
		}

		var result map[string]interface{}
		if err := c.ExecuteAndUnmarshal("ticket.status", params, &result); err != nil {
			return fmt.Errorf("failed to get ticket status: %w", err)
		}

		if ticketStatusJSON {
			output, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(output))
			return nil
		}

		// Format pending tickets
		fmt.Println("=== Pending Tickets ===")
		pendingRaw, _ := result["pending"].([]interface{})
		if len(pendingRaw) == 0 {
			fmt.Println("No pending tickets.")
		} else {
			fmt.Printf("%-15s %-12s %-8s %-10s %s\n", "ID", "TYPE", "PRIORITY", "BLOCKED", "TITLE")
			fmt.Println("-------------------------------------------------------------------------------")
			for _, ptRaw := range pendingRaw {
				pt, _ := ptRaw.(map[string]interface{})
				id, _ := pt["id"].(string)
				issueType, _ := pt["issue_type"].(string)
				priorityFloat, _ := pt["priority"].(float64)
				priority := int(priorityFloat)
				title, _ := pt["title"].(string)
				isBlocked, _ := pt["is_blocked"].(bool)
				blockedByRaw, _ := pt["blocked_by"].([]interface{})

				blocked := "-"
				if isBlocked {
					var blockers []string
					for _, b := range blockedByRaw {
						if bs, ok := b.(string); ok {
							blockers = append(blockers, bs)
						}
					}
					blocked = strings.Join(blockers, ",")
					if len(blocked) > 10 {
						blocked = blocked[:7] + "..."
					}
				}
				if len(title) > 35 {
					title = title[:32] + "..."
				}
				fmt.Printf("%-15s %-12s %-8d %-10s %s\n", id, issueType, priority, blocked, title)
			}
		}

		// Format completed tickets
		fmt.Println("\n=== Recently Completed ===")
		completedRaw, _ := result["completed"].([]interface{})
		if len(completedRaw) == 0 {
			fmt.Println("No completed tickets.")
		} else {
			fmt.Printf("%-15s %-12s %s\n", "ID", "TYPE", "TITLE")
			fmt.Println("---------------------------------------------------------------")
			for _, tRaw := range completedRaw {
				t, _ := tRaw.(map[string]interface{})
				id, _ := t["id"].(string)
				issueType, _ := t["issue_type"].(string)
				title, _ := t["title"].(string)
				if len(title) > 45 {
					title = title[:42] + "..."
				}
				fmt.Printf("%-15s %-12s %s\n", id, issueType, title)
			}
		}

		return nil
	},
}

func init() {
	// ticket create
	ticketCreateCmd.Flags().StringVar(&ticketCreateID, "id", "", "Custom ticket ID")
	ticketCreateCmd.Flags().StringVar(&ticketCreateTitle, "title", "", "Ticket title (required)")
	ticketCreateCmd.Flags().StringVar(&ticketCreateType, "type", "task", "Issue type (bug, feature, task, epic)")
	ticketCreateCmd.Flags().IntVar(&ticketCreatePriority, "priority", 2, "Priority (1=highest)")
	ticketCreateCmd.Flags().StringVarP(&ticketCreateDescription, "d", "d", "", "Ticket description")
	ticketCmd.AddCommand(ticketCreateCmd)

	// ticket list
	ticketListCmd.Flags().StringVar(&ticketListStatus, "status", "", "Filter by status")
	ticketListCmd.Flags().StringVar(&ticketListType, "type", "", "Filter by type")
	ticketListCmd.Flags().BoolVar(&ticketListJSON, "json", false, "Output in JSON format")
	ticketCmd.AddCommand(ticketListCmd)

	// ticket show
	ticketShowCmd.Flags().BoolVar(&ticketShowJSON, "json", false, "Output in JSON format")
	ticketCmd.AddCommand(ticketShowCmd)

	// ticket update
	ticketUpdateCmd.Flags().StringVar(&ticketUpdateTitle, "title", "", "New title")
	ticketUpdateCmd.Flags().StringVarP(&ticketUpdateDescription, "d", "d", "", "New description")
	ticketUpdateCmd.Flags().StringVar(&ticketUpdateStatus, "status", "", "New status")
	ticketUpdateCmd.Flags().IntVar(&ticketUpdatePriority, "priority", 0, "New priority")
	ticketUpdateCmd.Flags().StringVar(&ticketUpdateType, "type", "", "New type")
	ticketUpdateCmd.Flags().StringVar(&ticketUpdateAgentsState, "agents_state", "", "New agents state JSON")
	ticketCmd.AddCommand(ticketUpdateCmd)

	// ticket close
	ticketCloseCmd.Flags().StringVar(&ticketCloseReason, "reason", "", "Close reason")
	ticketCmd.AddCommand(ticketCloseCmd)

	// ticket delete
	ticketDeleteCmd.Flags().BoolVar(&ticketDeleteForce, "force", false, "Skip confirmation")
	ticketCmd.AddCommand(ticketDeleteCmd)

	// ticket search
	ticketSearchCmd.Flags().BoolVar(&ticketSearchJSON, "json", false, "Output in JSON format")
	ticketCmd.AddCommand(ticketSearchCmd)

	// ticket dep
	ticketDepCmd.AddCommand(ticketDepAddCmd)
	ticketDepCmd.AddCommand(ticketDepRemoveCmd)
	ticketCmd.AddCommand(ticketDepCmd)

	// ticket ready
	ticketReadyCmd.Flags().BoolVar(&ticketReadyJSON, "json", false, "Output in JSON format")
	ticketCmd.AddCommand(ticketReadyCmd)

	// ticket status
	ticketStatusCmd.Flags().IntVar(&ticketStatusPending, "pending", 20, "Number of pending tickets to show")
	ticketStatusCmd.Flags().IntVar(&ticketStatusCompleted, "completed", 15, "Number of completed tickets to show")
	ticketStatusCmd.Flags().BoolVar(&ticketStatusJSON, "json", false, "Output in JSON format")
	ticketCmd.AddCommand(ticketStatusCmd)
}
