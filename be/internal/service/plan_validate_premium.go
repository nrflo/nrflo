package service

import (
	"fmt"
	"strconv"
	"strings"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// PremiumWorkerCapKey is the config key (project override > global > default)
// capping how many plan nodes may resolve to a premium-tier model.
// DefaultDynwfMaxPremiumWorkers applies when the key is unset.
const (
	PremiumWorkerCapKey           = "dynwf_max_premium_workers"
	DefaultDynwfMaxPremiumWorkers = 2
)

// LoadDynwfMaxPremiumWorkers reads the premium-worker cap from the config KV
// (project override > global > default). Unlike SubworkflowCap (subworkflow.go),
// 0 is a valid, meaningful cap here (no premium nodes allowed at all) — only a
// missing, unparsable, or negative value falls back to the default.
func LoadDynwfMaxPremiumWorkers(pool *db.Pool, projectID string) int {
	raw, err := pool.GetProjectConfig(projectID, PremiumWorkerCapKey)
	if err != nil || raw == "" {
		raw, err = pool.GetConfig(PremiumWorkerCapKey)
		if err != nil || raw == "" {
			return DefaultDynwfMaxPremiumWorkers
		}
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 0 {
		return DefaultDynwfMaxPremiumWorkers
	}
	return n
}

// ModelTier is the cost class a plan node's bound model resolves to, for the
// premium-worker guardrail.
type ModelTier int

const (
	ModelTierCheap ModelTier = iota
	ModelTierMid
	ModelTierPremium
)

// PlanModelTierClass is the SINGLE place a model row is mapped to a cost
// tier (Rule 6 — polymorphism/classification lives in one place, never as
// scattered name-checks at call sites). It consults the registry's per-MTok
// pricing (model.PriceClass) when seeded; a row with no pricing (PriceIn
// NULL) falls back to name-class: the fable/opus family is premium, haiku is
// cheap, everything else (sonnet, gpt-*) is mid.
func PlanModelTierClass(m *model.Model) ModelTier {
	if tier, ok := m.PriceClass(); ok {
		switch tier {
		case model.PricePremium:
			return ModelTierPremium
		case model.PriceMid:
			return ModelTierMid
		default:
			return ModelTierCheap
		}
	}
	id := strings.ToLower(m.ID)
	switch {
	case strings.Contains(id, "opus"), strings.Contains(id, "fable"):
		return ModelTierPremium
	case strings.Contains(id, "haiku"):
		return ModelTierCheap
	default:
		return ModelTierMid
	}
}

// planNodeTier is one manifest node resolved to its bound template's model
// tier, in manifest order (layer 0's nodes first, then layer 1's, ...).
type planNodeTier struct {
	NodeID   string
	Template string
	Tier     ModelTier
}

// resolvePremiumNodes resolves every node's template -> model row -> tier, in
// manifest order. A node whose template does not resolve in the library is
// skipped here — ValidatePlanManifest is the authoritative check for that and
// always runs before EnforcePremiumWorkerCap at every call site.
func resolvePremiumNodes(pool *db.Pool, clk clock.Clock, projectID, workflowID string, m PlanManifest) ([]planNodeTier, error) {
	templates, err := AllowedTemplates(pool, projectID, workflowID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]PlanTemplate, len(templates))
	for _, t := range templates {
		byID[t.ID] = t
	}

	modelSvc := NewModelService(pool, clk)
	var out []planNodeTier
	for _, layer := range m.Layers {
		for _, node := range layer.Nodes {
			t, ok := byID[node.Template]
			if !ok {
				continue
			}
			row, err := modelSvc.Get(t.Model)
			if err != nil {
				continue
			}
			out = append(out, planNodeTier{NodeID: node.ID, Template: node.Template, Tier: PlanModelTierClass(row)})
		}
	}
	return out, nil
}

// cheapestNonPremiumTemplate picks the downgrade target for
// EnforcePremiumWorkerCap's auto-downgrade path: the library template with
// the lowest tier (cheap before mid), ties broken by id ascending.
func cheapestNonPremiumTemplate(pool *db.Pool, clk clock.Clock, projectID, workflowID string) (PlanTemplate, error) {
	templates, err := AllowedTemplates(pool, projectID, workflowID)
	if err != nil {
		return PlanTemplate{}, err
	}
	modelSvc := NewModelService(pool, clk)

	var best *PlanTemplate
	var bestTier ModelTier
	for i := range templates {
		row, err := modelSvc.Get(templates[i].Model)
		if err != nil {
			continue
		}
		tier := PlanModelTierClass(row)
		if tier == ModelTierPremium {
			continue
		}
		if best == nil || tier < bestTier || (tier == bestTier && templates[i].ID < best.ID) {
			t := templates[i]
			best = &t
			bestTier = tier
		}
	}
	if best == nil {
		return PlanTemplate{}, fmt.Errorf("no non-premium fanout_template is available in the template library to downgrade to")
	}
	return *best, nil
}

// EnforcePremiumWorkerCap resolves every node's bound-template tier and caps
// how many may be premium (dynwf_max_premium_workers, project>global>default).
//
// At or under cap: m is returned unchanged, warning is "". Over cap and
// canRevise=true: rejected with an error naming the offending nodes so the
// caller can replan (interactive Approve, reviseWithManifest). Over cap and
// canRevise=false (mode=auto): the earliest excess premium nodes are rebound
// to the cheapest non-premium library template — the LAST cap premium refs
// are left untouched, so the final result/synthesis node keeps its tier — and
// the modified manifest is returned alongside a non-empty warning.
func EnforcePremiumWorkerCap(pool *db.Pool, clk clock.Clock, projectID, workflowID string, m PlanManifest, canRevise bool) (PlanManifest, string, error) {
	maxPremium := LoadDynwfMaxPremiumWorkers(pool, projectID)

	nodeTiers, err := resolvePremiumNodes(pool, clk, projectID, workflowID, m)
	if err != nil {
		return m, "", err
	}
	var premiumIdx []int
	for i, nt := range nodeTiers {
		if nt.Tier == ModelTierPremium {
			premiumIdx = append(premiumIdx, i)
		}
	}
	if len(premiumIdx) <= maxPremium {
		return m, "", nil
	}

	if canRevise {
		names := make([]string, 0, len(premiumIdx))
		for _, i := range premiumIdx {
			names = append(names, nodeTiers[i].NodeID)
		}
		return m, "", fmt.Errorf(
			"plan binds %d nodes to premium-tier templates (%s), exceeding the cap of %d (%s); rebind the excess nodes to cheaper templates and resubmit",
			len(premiumIdx), strings.Join(names, ", "), maxPremium, PremiumWorkerCapKey)
	}

	target, err := cheapestNonPremiumTemplate(pool, clk, projectID, workflowID)
	if err != nil {
		return m, "", err
	}

	excess := len(premiumIdx) - maxPremium
	downgrade := make(map[string]bool, excess)
	for _, i := range premiumIdx[:excess] {
		downgrade[nodeTiers[i].NodeID] = true
	}

	out := m
	out.Layers = make([]PlanLayer, len(m.Layers))
	var downgraded []string
	for li, layer := range m.Layers {
		nodes := make([]PlanNode, len(layer.Nodes))
		for ni, node := range layer.Nodes {
			if downgrade[node.ID] {
				node.Template = target.ID
				downgraded = append(downgraded, node.ID)
			}
			nodes[ni] = node
		}
		out.Layers[li] = PlanLayer{Layer: layer.Layer, Policy: layer.Policy, Nodes: nodes}
	}

	warning := fmt.Sprintf(
		"plan bound %d nodes to premium-tier templates, exceeding the cap of %d (%s); downgraded %s to template %q",
		len(premiumIdx), maxPremium, PremiumWorkerCapKey, strings.Join(downgraded, ", "), target.ID)
	return out, warning, nil
}
