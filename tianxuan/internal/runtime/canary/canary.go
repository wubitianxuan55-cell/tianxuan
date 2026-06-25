package runtimecanary

type Evaluation struct{}
type BehaviorDiff struct {
	Attribution AttributionInfo
}
type BehaviorSample struct{}
type Policy struct{}
type AttributionInfo struct {
	PrimaryCause string
	Factors      []FactorInfo
}
type FactorInfo struct {
	Layer string
	Cause string
}
