package service

import (
	"be/internal/model"
	"be/internal/repo"
)

// AddDependency records that issueID is blocked by dependsOnID (a "blocks"
// dependency: dependsOnID must complete before issueID becomes runnable). Both
// tickets must already exist in the project (enforced by the repo).
func (s *TicketService) AddDependency(projectID, issueID, dependsOnID, createdBy string) error {
	if createdBy == "" {
		createdBy = "agent"
	}
	depRepo := repo.NewDependencyRepo(s.pool, s.clock)
	return depRepo.Create(&model.Dependency{
		ProjectID:   projectID,
		IssueID:     issueID,
		DependsOnID: dependsOnID,
		Type:        "blocks",
		CreatedBy:   createdBy,
	})
}
