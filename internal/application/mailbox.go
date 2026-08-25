package application

import (
	"context"
	"sync"
)

type mailRequest struct {
	ctx  context.Context
	fn   func(context.Context) (any, error)
	done chan mailResult
}
type mailResult struct {
	value any
	err   error
}
type mailbox struct{ queue chan mailRequest }

func newMailbox(capacity int) *mailbox {
	m := &mailbox{queue: make(chan mailRequest, capacity)}
	go m.run()
	return m
}

func (m *mailbox) run() {
	for request := range m.queue {
		if err := request.ctx.Err(); err != nil {
			request.done <- mailResult{err: err}
			continue
		}
		value, err := request.fn(request.ctx)
		request.done <- mailResult{value: value, err: err}
	}
}

func (m *mailbox) submit(ctx context.Context, fn func(context.Context) (any, error)) (any, error) {
	request := mailRequest{ctx: ctx, fn: fn, done: make(chan mailResult, 1)}
	select {
	case m.queue <- request:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case result := <-request.done:
		return result.value, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type mailboxRegistry struct {
	mu       sync.Mutex
	capacity int
	boxes    map[string]*mailbox
}

func newMailboxRegistry(capacity int) *mailboxRegistry {
	return &mailboxRegistry{capacity: capacity, boxes: make(map[string]*mailbox)}
}
func (r *mailboxRegistry) forDataset(id string) *mailbox {
	r.mu.Lock()
	defer r.mu.Unlock()
	if box := r.boxes[id]; box != nil {
		return box
	}
	box := newMailbox(r.capacity)
	r.boxes[id] = box
	return box
}
