package cli

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"be/internal/client"
	"be/internal/model"
)

// --- update ---

func init() {
	updateCmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a ticket",
		Args:  cobra.ExactArgs(1),
		RunE:  runTicketUpdate,
	}
	updateCmd.Flags().String("title", "", "New title")
	updateCmd.Flags().String("description", "", "New description")
	updateCmd.Flags().String("type", "", "New issue type")
	updateCmd.Flags().Int("priority", 0, "New priority (1-4)")
	updateCmd.Flags().String("parent", "", "New parent ticket ID (empty to clear)")
	ticketsCmd.AddCommand(updateCmd)
}

func runTicketUpdate(cmd *cobra.Command, args []string) error {
	if err := RequireProject(); err != nil {
		return err
	}
	c := getHTTPClient()

	body := map[string]interface{}{}
	if cmd.Flags().Changed("title") {
		v, _ := cmd.Flags().GetString("title")
		body["title"] = v
	}
	if cmd.Flags().Changed("description") {
		v, _ := cmd.Flags().GetString("description")
		body["description"] = v
	}
	if cmd.Flags().Changed("type") {
		v, _ := cmd.Flags().GetString("type")
		body["issue_type"] = v
	}
	if cmd.Flags().Changed("priority") {
		v, _ := cmd.Flags().GetInt("priority")
		body["priority"] = v
	}
	if cmd.Flags().Changed("parent") {
		v, _ := cmd.Flags().GetString("parent")
		body["parent_ticket_id"] = v
	}

	if len(body) == 0 {
		return fmt.Errorf("no fields to update")
	}

	var ticket model.Ticket
	if err := c.Patch("/api/v1/tickets/"+url.PathEscape(args[0]), body, &ticket); err != nil {
		return err
	}

	out, err := client.FormatTicketShow(&ticket, ticketsJSON)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// --- close ---

func init() {
	closeCmd := &cobra.Command{
		Use:   "close <id>",
		Short: "Close a ticket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireProject(); err != nil {
				return err
			}
			c := getHTTPClient()
			reason, _ := cmd.Flags().GetString("reason")

			var body interface{}
			if reason != "" {
				body = map[string]string{"reason": reason}
			}

			var ticket model.Ticket
			if err := c.Post("/api/v1/tickets/"+url.PathEscape(args[0])+"/close", body, &ticket); err != nil {
				return err
			}

			fmt.Printf("Ticket %s closed.\n", ticket.ID)
			return nil
		},
	}
	closeCmd.Flags().String("reason", "", "Close reason")
	ticketsCmd.AddCommand(closeCmd)
}

// --- reopen ---

func init() {
	reopenCmd := &cobra.Command{
		Use:   "reopen <id>",
		Short: "Reopen a closed ticket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireProject(); err != nil {
				return err
			}
			c := getHTTPClient()

			var ticket model.Ticket
			if err := c.Post("/api/v1/tickets/"+url.PathEscape(args[0])+"/reopen", struct{}{}, &ticket); err != nil {
				return err
			}

			fmt.Printf("Ticket %s reopened.\n", ticket.ID)
			return nil
		},
	}
	ticketsCmd.AddCommand(reopenCmd)
}

// --- delete ---

func init() {
	deleteCmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a ticket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireProject(); err != nil {
				return err
			}
			c := getHTTPClient()

			var resp json.RawMessage
			if err := c.Delete("/api/v1/tickets/"+url.PathEscape(args[0]), nil, &resp); err != nil {
				return err
			}

			fmt.Printf("Ticket %s deleted.\n", args[0])
			return nil
		},
	}
	ticketsCmd.AddCommand(deleteCmd)
}
