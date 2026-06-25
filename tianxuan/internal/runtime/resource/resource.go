package runtimeresource

type Reservation struct{}
type Decision struct{}
type CoordinatorSnapshot struct{}
type ResourceBudget struct{}

func NewCoordinator() *Coordinator { return &Coordinator{} }

type Coordinator struct{}
