package anticentralization

import (
	controlgraph "tianxuan/internal/controlplane/control_graph"
	globalstate "tianxuan/internal/equilibrium/global_state"
)

func Apply(decision controlgraph.ControlDecision, policy globalstate.EquilibriumPolicy, st globalstate.GlobalEquilibriumState) (controlgraph.ControlDecision, []string) {
	var adjustments []string
	if len(decision.NodeInfluence) == 0 {
		return decision, adjustments
	}
	totalInfluence := 0.0
	for _, influence := range decision.NodeInfluence {
		totalInfluence += influence.Share
	}
	if totalInfluence <= 0 {
		return decision, adjustments
	}
	dominantShare := 0.0
	dominantNode := ""
	for _, influence := range decision.NodeInfluence {
		share := influence.Share / totalInfluence
		if share > dominantShare {
			dominantShare = share
			dominantNode = influence.NodeID
		}
	}
	if dominantShare > 0.60 {
		adjusted := make([]controlgraph.NodeInfluence, len(decision.NodeInfluence))
		for i, influence := range decision.NodeInfluence {
			adjusted[i] = influence
			if influence.NodeID == dominantNode {
				adjusted[i].Weight *= 0.75
				adjusted[i].Share = dominantShare * 0.80
			} else {
				adjusted[i].Weight *= 1.12
			}
		}
		decision.NodeInfluence = adjusted
		adjustments = append(adjustments, "anti-centralization: redistributed weight from "+dominantNode)
	}
	if st.ControlGraphEntropy < 0.50 && dominantShare > 0.45 {
		decision.ExplorationRatePercent = controlgraph.MaxExplorationRatePercent
		adjustments = append(adjustments, "anti-centralization: increased exploration rate due to low entropy")
	}
	return decision, adjustments
}
