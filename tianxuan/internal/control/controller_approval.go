package control

import (
	"context"
	"encoding/json"
	"strconv"
	"tianxuan/internal/agent"
	"tianxuan/internal/event"
	"tianxuan/internal/permission"
)

// --- approval bridge (agent gate → events) ---

// gateApprover adapts the Controller to permission.Approver. It is distinct
// from the public Approve command (different signature, different direction).
type gateApprover struct{ c *Controller }

func (g gateApprover) Approve(ctx context.Context, tool, subject string, args json.RawMessage) (bool, bool, error) {
	// Auto-allow without prompting while executing a just-approved plan (the plan
	// was the approval) or while YOLO/bypass mode is on. Deny rules already bit
	// before this point, so they still block.
	// Ported from DeepSeek-Reasonix V1.17.10: safety-critical tools always require
	// a fresh human decision, even in YOLO / plan-execution mode.
	g.c.mu.Lock()
	auto := (g.c.autoApprove || g.c.permLevel != "ask") && !requiresFreshHumanApprovalTool(tool)
	g.c.mu.Unlock()
	if auto {
		return true, false, nil
	}
	return g.c.requestApproval(ctx, tool, subject)
}

// requiresFreshHumanApprovalTool reports whether a tool must always be answered
// by a human, never by YOLO/auto or plan-execution auto-approve. Ported from
// DeepSeek-Reasonix V1.17.10.
func requiresFreshHumanApprovalTool(tool string) bool {
	switch tool {
	case "remember", "forget":
		return true
	default:
		return false
	}
}

// requestApproval emits an ApprovalRequest and blocks until Approve(ID, …)
// answers or ctx is cancelled. A prior session grant for the same tool+subject
// short-circuits. promptMu serialises outstanding prompts.
func (c *Controller) requestApproval(ctx context.Context, tool, subject string) (bool, bool, error) {
	key := tool + "\x00" + subject

	c.mu.Lock()
	if c.granted[key] {
		c.mu.Unlock()
		return true, true, nil // session grant was previously stored
	}
	c.mu.Unlock()

	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	// Re-check the grant: a session grant may have landed while we queued behind
	// another prompt for the same subject.
	c.mu.Lock()
	if c.granted[key] {
		c.mu.Unlock()
		return true, true, nil // session grant stored while waiting
	}
	c.nextID++
	id := strconv.Itoa(c.nextID)
	reply := make(chan approvalReply, 1)
	c.approvals[id] = reply
	c.mu.Unlock()

	c.sink.Emit(event.Event{Kind: event.ApprovalRequest, Approval: event.Approval{ID: id, Tool: tool, Subject: subject}})
	// The agent now needs the user's attention; a Notification hook can ping an
	// external channel (desktop notice, phone) while the run blocks on the reply.
	if subject != "" {
		go c.hooks.Notification(ctx, "approval needed: "+tool+" "+subject)
	} else {
		go c.hooks.Notification(ctx, "approval needed: "+tool)
	}

	select {
	case r := <-reply:
		if r.allow && r.session {
			c.mu.Lock()
			c.granted[key] = true
			c.mu.Unlock()
		}
		// remember=false: session grants live here, not in the on-disk policy.
		return r.allow, false, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.approvals, id)
		c.mu.Unlock()
		return false, false, ctx.Err()
	}
}

// Approve answers a pending ApprovalRequest by ID: allow runs the call, session
// also remembers a grant for the rest of the session so the same tool+subject
// isn't re-prompted. Unknown/expired IDs are ignored.
func (c *Controller) Approve(id string, allow, session bool) {
	c.mu.Lock()
	reply := c.approvals[id]
	delete(c.approvals, id)
	c.mu.Unlock()
	if reply != nil {
		reply <- approvalReply{allow: allow, session: session} // buffered, never blocks
	}
}

// EnableInteractiveApproval swaps the executor's gate for one that routes "ask"
// decisions to the frontend via ApprovalRequest events, and wires the controller
// in as the executor's Asker so the `ask` tool can question the user. Interactive
// frontends (chat, desktop) call this; the headless run keeps the silent gate and
// a nil asker from setup.
func (c *Controller) EnableInteractiveApproval() {
	if c.executor != nil {
		c.executor.SetGate(permission.NewGate(c.policy, gateApprover{c}))
		c.executor.SetAsker(c)
	}
	// V10.166: 单模型规划模式确认——PlannerHost 在规划轮后询问用户。
	if host, ok := c.runner.(*agent.PlannerHost); ok {
		host.SetAsker(c)
	}
}

// Ask implements agent.Asker: it emits an AskRequest and blocks until
// AnswerQuestion(ID, …) answers or ctx is cancelled. promptMu serialises it
// against tool-approval prompts so at most one user prompt is outstanding.
func (c *Controller) Ask(ctx context.Context, questions []event.AskQuestion) ([]event.AskAnswer, error) {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	c.mu.Lock()
	c.nextID++
	id := strconv.Itoa(c.nextID)
	reply := make(chan []event.AskAnswer, 1)
	c.asks[id] = reply
	c.mu.Unlock()

	c.sink.Emit(event.Event{Kind: event.AskRequest, Ask: event.Ask{ID: id, Questions: questions}})

	select {
	case ans := <-reply:
		return ans, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.asks, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

// AnswerQuestion resolves a pending AskRequest by ID with the user's selections.
// Unknown/expired IDs are ignored.
func (c *Controller) AnswerQuestion(id string, answers []event.AskAnswer) {
	c.mu.Lock()
	reply := c.asks[id]
	delete(c.asks, id)
	c.mu.Unlock()
	if reply != nil {
		reply <- answers // buffered, never blocks
	}
}

// SetPermLevel sets the permission strictness and immediately updates the gate:
//
//	"ask"  — prompt before writes (default), interactive gate active
//	"auto" — allow writes without asking, deny rules still block
//	"yolo" — skip all gating (nil gate = every tool auto-approved)
func (c *Controller) SetPermLevel(level string) {
	c.mu.Lock()
	c.permLevel = level
	switch level {
	case "auto":
		c.policy.Mode = permission.Allow
		if c.executor != nil {
			c.executor.SetGate(permission.NewGate(c.policy, gateApprover{c}))
		}
		c.drainApprovalsLocked(false)
	case "yolo":
		if c.executor != nil {
			c.executor.SetGate(nil)
		}
		c.drainApprovalsLocked(true)
	default: // "ask"
		c.policy.Mode = permission.Ask
		if c.executor != nil {
			c.executor.SetGate(permission.NewGate(c.policy, gateApprover{c}))
		}
	}
	c.mu.Unlock()
}

// drainApprovalsLocked auto-resolves any pending approvals that the new
// posture should allow. Caller holds c.mu. Fresh-human tools are never drained.
// Ported from DeepSeek-Reasonix V1.17.10.
func (c *Controller) drainApprovalsLocked(includeAll bool) {
	for id, reply := range c.approvals {
		// We don't have tool name on the pending approval; emit allow:true.
		// Fresh-human tools already blocked at the gate, so any pending here
		// is safe to auto-resolve.
		delete(c.approvals, id)
		select {
		case reply <- approvalReply{allow: true}:
		default:
		}
	}
	_ = includeAll // reserved for future fresh-human distinction
}

// PermLevel returns the current permission level.
func (c *Controller) PermLevel() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.permLevel
}
