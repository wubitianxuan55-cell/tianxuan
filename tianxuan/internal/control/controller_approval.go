package control

import (
	"context"
	"encoding/json"
	"strconv"

	"tianxuan/internal/event"
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
