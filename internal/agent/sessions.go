package agent

import (
	"encoding/json"
	"os"
	"sync"
)

// SessionStore is the zeroclaw-owned mapping from conversation names to
// backend session ids. It is deliberately host-side state: the agent's
// harness can be swapped or its environment rebuilt without zeroclaw
// forgetting which conversation is which.
type SessionStore struct {
	path string
	mu   sync.Mutex
	m    map[string]string
}

func OpenSessionStore(path string) (*SessionStore, error) {
	s := &SessionStore{path: path, m: map[string]string{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.m); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *SessionStore) Get(conversation string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[conversation]
}

func (s *SessionStore) Set(conversation, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[conversation] = sessionID
	data, err := json.MarshalIndent(s.m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

// Delete drops a conversation's session mapping. The next turn starts a fresh
// backend session under the same conversation name.
func (s *SessionStore) Delete(conversation string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[conversation]; !ok {
		return nil
	}
	delete(s.m, conversation)
	data, err := json.MarshalIndent(s.m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func (s *SessionStore) All() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return out
}
