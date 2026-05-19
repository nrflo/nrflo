package service

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// ErrObserverDisabled is returned by Launch when experimental_observer_enabled is false.
var ErrObserverDisabled = errors.New("observer_disabled")

// ObserverSpawnRequest carries all inputs for SpawnObserver.
type ObserverSpawnRequest struct {
	SessionID      string
	SpawnToken     string
	Scope          string // "workflow", "project", or "global"
	ProjectID      string
	WorkflowID     string // workflow definition ID (empty for global scope)
	SystemContext  string // resolved system context prompt
	DynamicContext string // assembled dynamic markdown bundle
	Provider       string // resolved provider override (empty = default)
	Model          string // resolved model override (empty = default)
}

// ObserverSpawner is the minimal interface that Spawner implements to start observer agents.
// Defined here to avoid a circular import (spawner → service already exists).
type ObserverSpawner interface {
	SpawnObserver(req ObserverSpawnRequest) error
}

// ObserverService manages observer agent lifecycle.
type ObserverService struct {
	pool               *db.Pool
	clock              clock.Clock
	globalSettings     *GlobalSettingsService
	workflowSvc        *WorkflowService
	agentSvc           *AgentService
	findingsSvc        *FindingsService
	projectFindingsSvc *ProjectFindingsService
	projectSvc         *ProjectService
	spawner            ObserverSpawner
}

// NewObserverService creates a new ObserverService.
func NewObserverService(
	pool *db.Pool,
	clk clock.Clock,
	globalSettings *GlobalSettingsService,
	workflowSvc *WorkflowService,
	agentSvc *AgentService,
	findingsSvc *FindingsService,
	projectFindingsSvc *ProjectFindingsService,
	projectSvc *ProjectService,
	spawner ObserverSpawner,
) *ObserverService {
	return &ObserverService{
		pool:               pool,
		clock:              clk,
		globalSettings:     globalSettings,
		workflowSvc:        workflowSvc,
		agentSvc:           agentSvc,
		findingsSvc:        findingsSvc,
		projectFindingsSvc: projectFindingsSvc,
		projectSvc:         projectSvc,
		spawner:            spawner,
	}
}

// Resolve returns (sysCtx, provider, model) by merging global → project → workflow overrides.
// Each field is resolved independently: workflow override wins, then project, then global.
func (s *ObserverService) Resolve(scope, projectID, workflowID string) (sysCtx, provider, mdl string, err error) {
	sysCtx, err = s.globalSettings.GetObserverSystemContext()
	if err != nil {
		return "", "", "", fmt.Errorf("get global observer system context: %w", err)
	}
	provider, err = s.globalSettings.GetObserverProvider()
	if err != nil {
		return "", "", "", fmt.Errorf("get global observer provider: %w", err)
	}
	mdl, err = s.globalSettings.GetObserverModel()
	if err != nil {
		return "", "", "", fmt.Errorf("get global observer model: %w", err)
	}

	if projectID != "" {
		if v, e := s.globalSettings.GetObserverSystemContextForProject(projectID); e == nil && v != "" {
			sysCtx = v
		}
		if v, e := s.globalSettings.GetObserverProviderForProject(projectID); e == nil && v != "" {
			provider = v
		}
		if v, e := s.globalSettings.GetObserverModelForProject(projectID); e == nil && v != "" {
			mdl = v
		}
	}

	if scope == "workflow" && workflowID != "" && projectID != "" {
		wfRepo := repo.NewWorkflowRepo(s.pool, s.clock)
		wf, e := wfRepo.Get(projectID, workflowID)
		if e == nil {
			if wf.ObserverContext != "" {
				sysCtx = wf.ObserverContext
			}
			if wf.ObserverProvider.Valid && wf.ObserverProvider.String != "" {
				provider = wf.ObserverProvider.String
			}
			if wf.ObserverModel.Valid && wf.ObserverModel.String != "" {
				mdl = wf.ObserverModel.String
			}
		}
	}

	return sysCtx, provider, mdl, nil
}

// Launch starts an observer session. Returns ErrObserverDisabled when the feature flag is off.
func (s *ObserverService) Launch(scope, projectID, workflowID string) (sessionID string, err error) {
	enabled, err := s.globalSettings.GetExperimentalObserverEnabled()
	if err != nil {
		return "", fmt.Errorf("check observer enabled: %w", err)
	}
	if !enabled {
		return "", ErrObserverDisabled
	}

	sysCtx, provider, mdl, err := s.Resolve(scope, projectID, workflowID)
	if err != nil {
		return "", fmt.Errorf("resolve observer config: %w", err)
	}

	dynamicCtx, err := AssembleDynamicContext(s, scope, projectID, workflowID)
	if err != nil {
		return "", fmt.Errorf("assemble dynamic context: %w", err)
	}

	sessionID = uuid.New().String()
	spawnToken := mintObserverToken()
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)

	sessionRepo := repo.NewAgentSessionRepo(s.pool, s.clock)
	sess := &model.AgentSession{
		ID:            sessionID,
		ProjectID:     projectID,
		TicketID:      "",
		AgentType:     "_observer",
		Phase:         "observer",
		Status:        model.AgentSessionRunning,
		Kind:          "observer",
		ObserverScope:      sql.NullString{String: scope, Valid: scope != ""},
		ObserverWorkflowID: sql.NullString{String: workflowID, Valid: scope == "workflow" && workflowID != ""},
		SpawnToken:         sql.NullString{String: spawnToken, Valid: true},
		StartedAt:          sql.NullString{String: now, Valid: true},
	}
	if err := sessionRepo.Create(sess); err != nil {
		return "", fmt.Errorf("create observer session: %w", err)
	}

	spawnReq := ObserverSpawnRequest{
		SessionID:      sessionID,
		SpawnToken:     spawnToken,
		Scope:          scope,
		ProjectID:      projectID,
		WorkflowID:     workflowID,
		SystemContext:  sysCtx,
		DynamicContext: dynamicCtx,
		Provider:       provider,
		Model:          mdl,
	}
	if err := s.spawner.SpawnObserver(spawnReq); err != nil {
		_ = sessionRepo.UpdateStatusToFailedWithReason(sessionID, "spawn_failed")
		return "", fmt.Errorf("spawn observer: %w", err)
	}

	return sessionID, nil
}

func mintObserverToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return uuid.New().String() + uuid.New().String()
	}
	return hex.EncodeToString(buf)
}
