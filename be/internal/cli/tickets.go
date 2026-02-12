package cli

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"be/internal/client"
	"be/internal/model"
)

var (
	ticketsServer string
	ticketsJSON   bool
)

func init() {
	rootCmd.AddCommand(ticketsCmd)

	// Common flags
	ticketsCmd.PersistentFlags().StringVar(&ticketsServer, "server", "", "API server URL (default: NRWORKFLOW_API_URL or http://localhost:6587)")
	ticketsCmd.PersistentFlags().BoolVar(&ticketsJSON, "json", false, "Output as JSON")
}

func getHTTPClient() *client.HTTPClient {
	baseURL := ticketsServer
	if baseURL == "" {
		baseURL = os.Getenv("NRWORKFLOW_API_URL")
	}
	if baseURL == "" {
		baseURL = "http://localhost:6587"
	}
	return client.NewHTTPClient(baseURL, ProjectID)
}

var ticketsCmd = &cobra.Command{
	Use:   "tickets",
	Short: "Ticket management commands",
}

// --- list ---

var (
	ticketListStatus string
	ticketListType   string
	ticketListParent string
)

func init() {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List tickets",
		RunE:  runTicketList,
	}
	listCmd.Flags().StringVar(&ticketListStatus, "status", "", "Filter by status (open, in_progress, closed, blocked)")
	listCmd.Flags().StringVar(&ticketListType, "type", "", "Filter by type (bug, feature, task, epic)")
	listCmd.Flags().StringVar(&ticketListParent, "parent", "", "Filter by parent ticket ID")
	ticketsCmd.AddCommand(listCmd)
}

func runTicketList(cmd *cobra.Command, args []string) error {
	if err := RequireProject(); err != nil {
		return err
	}
	c := getHTTPClient()

	params := url.Values{}
	if ticketListStatus != "" {
		params.Set("status", ticketListStatus)
	}
	if ticketListType != "" {
		params.Set("type", ticketListType)
	}

	path := "/api/v1/tickets"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp struct {
		Tickets []*model.Ticket `json:"tickets"`
	}
	if err := c.Get(path, &resp); err != nil {
		return err
	}

	// Filter by parent client-side if specified
	if ticketListParent != "" {
		filtered := make([]*model.Ticket, 0)
		for _, t := range resp.Tickets {
			if t.ParentTicketID.Valid && strings.EqualFold(t.ParentTicketID.String, ticketListParent) {
				filtered = append(filtered, t)
			}
		}
		resp.Tickets = filtered
	}

	out, err := client.FormatTicketList(resp.Tickets, ticketsJSON)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// --- get ---

func init() {
	getCmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a ticket by ID",
		Args:  cobra.ExactArgs(1),
		RunE:  runTicketGet,
	}
	ticketsCmd.AddCommand(getCmd)
}

func runTicketGet(cmd *cobra.Command, args []string) error {
	if err := RequireProject(); err != nil {
		return err
	}
	c := getHTTPClient()

	var ticket model.Ticket
	if err := c.Get("/api/v1/tickets/"+url.PathEscape(args[0]), &ticket); err != nil {
		return err
	}

	out, err := client.FormatTicketShow(&ticket, ticketsJSON)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// --- create ---

var (
	ticketCreateTitle       string
	ticketCreateDescription string
	ticketCreateType        string
	ticketCreatePriority    int
	ticketCreateParent      string
	ticketCreateCreatedBy   string
)

func init() {
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new ticket",
		RunE:  runTicketCreate,
	}
	createCmd.Flags().StringVar(&ticketCreateTitle, "title", "", "Ticket title (required)")
	createCmd.Flags().StringVar(&ticketCreateDescription, "description", "", "Ticket description")
	createCmd.Flags().StringVar(&ticketCreateType, "type", "task", "Issue type (bug, feature, task, epic)")
	createCmd.Flags().IntVar(&ticketCreatePriority, "priority", 2, "Priority (1-4)")
	createCmd.Flags().StringVar(&ticketCreateParent, "parent", "", "Parent ticket ID")
	createCmd.Flags().StringVar(&ticketCreateCreatedBy, "created-by", "cli", "Created by")
	_ = createCmd.MarkFlagRequired("title")
	ticketsCmd.AddCommand(createCmd)
}

func runTicketCreate(cmd *cobra.Command, args []string) error {
	if err := RequireProject(); err != nil {
		return err
	}
	c := getHTTPClient()

	body := map[string]interface{}{
		"title":      ticketCreateTitle,
		"issue_type": ticketCreateType,
		"priority":   ticketCreatePriority,
		"created_by": ticketCreateCreatedBy,
	}
	if ticketCreateDescription != "" {
		body["description"] = ticketCreateDescription
	}
	if ticketCreateParent != "" {
		body["parent_ticket_id"] = ticketCreateParent
	}

	var ticket model.Ticket
	if err := c.Post("/api/v1/tickets", body, &ticket); err != nil {
		return err
	}

	out, err := client.FormatTicketShow(&ticket, ticketsJSON)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}
