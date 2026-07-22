package api

import (
	"net/http"
	"strconv"

	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

func (s *Server) tierModelService() *service.TierModelService {
	return service.NewTierModelService(s.pool, s.clock, service.NewModelService(s.pool, s.clock))
}

// registerTierModelRoutes registers the tier_models (global) routes. Split
// out of server.go to keep that file's line count under its baseline
// (mirrors registerCustomProviderRoutes).
func (s *Server) registerTierModelRoutes(protected, admin func(string, http.HandlerFunc)) {
	protected("GET /api/v1/tier-models", s.handleListTierModels)
	admin("PUT /api/v1/tier-models/{tier}", s.handleSetTierChain)
}

// handleListTierModels returns every tier's ordered fallback chain.
func (s *Server) handleListTierModels(w http.ResponseWriter, r *http.Request) {
	svc := s.tierModelService()

	tiers, err := svc.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, tiers)
}

// handleSetTierChain replaces a tier's ordered fallback chain.
func (s *Server) handleSetTierChain(w http.ResponseWriter, r *http.Request) {
	tier, err := strconv.Atoi(r.PathValue("tier"))
	if err != nil || tier < 1 || tier > 5 {
		writeError(w, http.StatusBadRequest, "tier must be an integer between 1 and 5")
		return
	}

	var req types.SetTierChainRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := s.tierModelService()

	if err := svc.SetTierChain(tier, req.Entries); err != nil {
		writeServiceError(w, err)
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventTierModelsUpdated, "", "", "", map[string]interface{}{
			"tier": tier,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
