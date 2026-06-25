package semanticrouter

import (
	"errors"

	controltypes "tianxuan/internal/controlsemantics/types"
)

// Route processes a single typed signal through the semantic router.
// Returns the signal unchanged on success, or an error if the signal is invalid.
func Route(sig controltypes.TypedSignal) (controltypes.TypedSignal, error) {
	if sig.Type == controltypes.SignalDecision && sig.Payload == nil {
		return sig, errors.New("decision signal with nil payload rejected")
	}
	return sig, nil
}

// RouteLayer processes a batch of signals for a given control layer.
func RouteLayer(layer controltypes.Layer, signals []controltypes.TypedSignal) ([]controltypes.TypedSignal, error) {
	if len(signals) == 0 {
		return nil, nil
	}
	var out []controltypes.TypedSignal
	for _, sig := range signals {
		if _, err := Route(sig); err != nil {
			return nil, err
		}
		if sig.Type == controltypes.SignalConstraint || sig.Rationale != "" {
			out = append(out, sig)
		}
	}
	return out, nil
}
