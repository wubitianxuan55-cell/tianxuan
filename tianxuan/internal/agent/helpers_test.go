package agent

import (
	"context"

	"tianxuan/internal/provider"
)

// mockProvider replays preset chunks and records the last request it received.
// Used across agent tests (dispatch_test, guards_test, planmode_test, task_test).
type mockProvider struct {
	name    string
	chunks  []provider.Chunk
	lastReq provider.Request
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	m.lastReq = req
	ch := make(chan provider.Chunk, len(m.chunks))
	for _, c := range m.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

// lastUser returns the content of the last user-role message in a request.
func lastUser(req provider.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == provider.RoleUser {
			return req.Messages[i].Content
		}
	}
	return ""
}
