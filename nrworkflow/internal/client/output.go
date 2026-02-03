package client

import (
	"encoding/json"
	"fmt"
	"strings"

	"nrworkflow/internal/model"
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
	sb.WriteString(fmt.Sprintf("%-15s %-12s %-10s %-8s %s\n", "ID", "TYPE", "STATUS", "PRIORITY", "TITLE"))
	sb.WriteString("-------------------------------------------------------------------------------\n")
	for _, t := range tickets {
		title := t.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		sb.WriteString(fmt.Sprintf("%-15s %-12s %-10s %-8d %s\n", t.ID, t.IssueType, t.Status, t.Priority, title))
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
	sb.WriteString(fmt.Sprintf("ID:          %s\n", ticket.ID))
	sb.WriteString(fmt.Sprintf("Project:     %s\n", ticket.ProjectID))
	sb.WriteString(fmt.Sprintf("Title:       %s\n", ticket.Title))
	sb.WriteString(fmt.Sprintf("Type:        %s\n", ticket.IssueType))
	sb.WriteString(fmt.Sprintf("Status:      %s\n", ticket.Status))
	sb.WriteString(fmt.Sprintf("Priority:    %d\n", ticket.Priority))
	sb.WriteString(fmt.Sprintf("Created By:  %s\n", ticket.CreatedBy))
	sb.WriteString(fmt.Sprintf("Created:     %s\n", ticket.CreatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("Updated:     %s\n", ticket.UpdatedAt.Format("2006-01-02 15:04:05")))

	if ticket.Description.Valid && ticket.Description.String != "" {
		sb.WriteString(fmt.Sprintf("\nDescription:\n%s\n", ticket.Description.String))
	}

	if ticket.AgentsState.Valid && ticket.AgentsState.String != "" {
		sb.WriteString(fmt.Sprintf("\nAgents State:\n%s\n", ticket.AgentsState.String))
	}

	return sb.String(), nil
}

// FormatProjectList formats a list of projects for display
func FormatProjectList(projects []*model.Project, jsonOutput bool) (string, error) {
	if jsonOutput {
		data, err := json.MarshalIndent(projects, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	if len(projects) == 0 {
		return "No projects found.\n\nCreate one with: nrworkflow project create <project-id>", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-20s %-30s %-15s\n", "ID", "NAME", "WORKFLOW"))
	sb.WriteString(strings.Repeat("-", 70) + "\n")
	for _, p := range projects {
		workflow := "-"
		if p.DefaultWorkflow.Valid {
			workflow = p.DefaultWorkflow.String
		}
		name := p.Name
		if len(name) > 28 {
			name = name[:25] + "..."
		}
		sb.WriteString(fmt.Sprintf("%-20s %-30s %-15s\n", p.ID, name, workflow))
	}
	return sb.String(), nil
}

// FormatProjectShow formats a single project for display
func FormatProjectShow(project *model.Project, jsonOutput bool) (string, error) {
	if jsonOutput {
		data, err := json.MarshalIndent(project, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ID:               %s\n", project.ID))
	sb.WriteString(fmt.Sprintf("Name:             %s\n", project.Name))
	if project.RootPath.Valid {
		sb.WriteString(fmt.Sprintf("Root Path:        %s\n", project.RootPath.String))
	}
	if project.DefaultWorkflow.Valid {
		sb.WriteString(fmt.Sprintf("Default Workflow: %s\n", project.DefaultWorkflow.String))
	}
	sb.WriteString(fmt.Sprintf("Created:          %s\n", project.CreatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("Updated:          %s\n", project.UpdatedAt.Format("2006-01-02 15:04:05")))

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
