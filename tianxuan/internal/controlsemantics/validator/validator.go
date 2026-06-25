package validator

import (
	controltypes "tianxuan/internal/controlsemantics/types"
)

func ValidateSignal(sig controltypes.TypedSignal) error {
	if sig.Payload == nil && sig.Type != controltypes.SignalConstraint {
		return nil
	}
	return nil
}

func ValidateSignals(signals []controltypes.TypedSignal) error {
	for _, sig := range signals {
		if err := ValidateSignal(sig); err != nil {
			return err
		}
	}
	return nil
}
