package agent

import "fmt"

// Guard checks whether an agent should continue execution.
type Guard interface {
	// Check returns an error if execution should stop.
	Check() error
	// Name returns the guard's identifier.
	Name() string
}

// BudgetExceededError is returned when a guard's budget is exceeded.
type BudgetExceededError struct {
	GuardName string
	Message   string
}

func (e *BudgetExceededError) Error() string {
	return fmt.Sprintf("%s: %s", e.GuardName, e.Message)
}

// CompositeGuard combines multiple guards with optional parent cascading.
type CompositeGuard struct {
	guards []Guard
	parent *CompositeGuard
}

// NewCompositeGuard creates a composite guard with optional parent.
func NewCompositeGuard(parent *CompositeGuard, guards ...Guard) *CompositeGuard {
	return &CompositeGuard{
		guards: guards,
		parent: parent,
	}
}

// Check runs all local guards, then cascades to parent.
func (cg *CompositeGuard) Check() error {
	for _, g := range cg.guards {
		if err := g.Check(); err != nil {
			return err
		}
	}
	if cg.parent != nil {
		return cg.parent.Check()
	}
	return nil
}

// Name returns "composite".
func (cg *CompositeGuard) Name() string {
	return "composite"
}

// Add appends a guard to this composite.
func (cg *CompositeGuard) Add(g Guard) {
	cg.guards = append(cg.guards, g)
}

// AddTokens propagates token usage to every TokenGuard in the chain
// (local guards and parent). This ensures that a shared root TokenGuard
// accumulates usage from all child agents.
func (cg *CompositeGuard) AddTokens(in, out int, costIn, costOut float64) {
	for _, g := range cg.guards {
		if tg, ok := g.(*TokenGuard); ok {
			tg.Add(in, out, costIn, costOut)
		}
	}
	if cg.parent != nil {
		cg.parent.AddTokens(in, out, costIn, costOut)
	}
}
