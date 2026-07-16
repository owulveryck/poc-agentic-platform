package store

import "sync"

// Memory is an in-memory implementation of TokenStore + SessionStore for
// tests. Safe for concurrent use.
type Memory struct {
	mu      sync.Mutex
	active  string
	tickets map[string]string
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{tickets: map[string]string{}}
}

// Put implements TokenStore.
func (m *Memory) Put(sessionID, ticket string) error {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickets[sessionID] = ticket
	return nil
}

// Get implements TokenStore.
func (m *Memory) Get(sessionID string) (string, error) {
	if err := validateSessionID(sessionID); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tickets[sessionID]
	if !ok {
		return "", ErrNotFound
	}
	return t, nil
}

// Delete implements TokenStore.
func (m *Memory) Delete(sessionID string) error {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tickets, sessionID)
	return nil
}

// Reset implements TokenStore.
func (m *Memory) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickets = map[string]string{}
	return nil
}

// PutActive implements SessionStore.
func (m *Memory) PutActive(sessionID string) error {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = sessionID
	return nil
}

// GetActive implements SessionStore.
func (m *Memory) GetActive() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == "" {
		return "", ErrNotFound
	}
	return m.active, nil
}

// ClearActive implements SessionStore.
func (m *Memory) ClearActive() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = ""
	return nil
}
