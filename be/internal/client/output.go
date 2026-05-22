package client

import (
	"encoding/json"
	"fmt"
	"strings"

	"be/internal/model"
)

// FormatTicketList formats a list of tickets for display
func FormatTicketList(tickets []*model.Ticket, jsonOutput bool) (string, error) {
	if jsonOutput {
		data, err := json.MarshalIndent(tickets, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	if len(tickets) == 0 {
		return "No tickets found.", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%-15s %-12s %-10s %-8s %s\n", "ID", "TYPE", "STATUS", "PRIORITY", "TITLE")
	sb.WriteString("-------------------------------------------------------------------------------\n")
	for _, t := range tickets {
		title := t.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		fmt.Fprintf(&sb, "%-15s %-12s %-10s %-8d %s\n", t.ID, t.IssueType, t.Status, t.Priority, title)
	}
	return sb.String(), nil
}

// FormatTicketShow formats a single ticket for display
func FormatTicketShow(ticket *model.Ticket, jsonOutput bool) (string, error) {
	if jsonOutput {
		data, err := json.MarshalIndent([]*model.Ticket{ticket}, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "ID:          %s\n", ticket.ID)
	fmt.Fprintf(&sb, "Project:     %s\n", ticket.ProjectID)
	fmt.Fprintf(&sb, "Title:       %s\n", ticket.Title)
	fmt.Fprintf(&sb, "Type:        %s\n", ticket.IssueType)
	fmt.Fprintf(&sb, "Status:      %s\n", ticket.Status)
	fmt.Fprintf(&sb, "Priority:    %d\n", ticket.Priority)
	fmt.Fprintf(&sb, "Created By:  %s\n", ticket.CreatedBy)
	fmt.Fprintf(&sb, "Created:     %s\n", ticket.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&sb, "Updated:     %s\n", ticket.UpdatedAt.Format("2006-01-02 15:04:05"))

	if ticket.Description.Valid && ticket.Description.String != "" {
		fmt.Fprintf(&sb, "\nDescription:\n%s\n", ticket.Description.String)
	}

	return sb.String(), nil
}

// FormatJSON formats any value as indented JSON
func FormatJSON(v interface{}) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FormatValue formats a value - either as JSON or as string
func FormatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case nil:
		return "null"
	default:
		data, _ := json.MarshalIndent(v, "", "  ")
		return string(data)
	}
}
