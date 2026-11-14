// Package session stores chat sessions as local JSON files (the PC is the
// backend — no cloud service).
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mr-daaku/opentitam/internal/config"
	"github.com/mr-daaku/opentitam/internal/provider"
)

// Session is one persisted conversation.
type Session struct {
	ID        string             `json:"id"`
	Title     string             `json:"title"`
	Model     string             `json:"model"`
	Provider  string             `json:"provider"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
	Messages  []provider.Message `json:"messages"`
}

// Store persists sessions under the local data dir.
type Store struct {
	Dir string
}

// NewStore creates the store (data dir / sessions).
func NewStore() *Store {
	dir := filepath.Join(config.DataDir(), "sessions")
	_ = os.MkdirAll(dir, 0o755)
	return &Store{Dir: dir}
}

func (s *Store) path(id string) string {
	return filepath.Join(s.Dir, id+".json")
}

// New creates an empty session with a unique ID.
func (s *Store) New(model, providerName string) *Session {
	now := time.Now()
	id := fmt.Sprintf("s_%d", now.UnixNano())
	return &Session{
		ID:        id,
		Title:     "New session",
		Model:     model,
		Provider:  providerName,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Save writes the session to disk.
func (s *Store) Save(sess *Session) error {
	sess.UpdatedAt = time.Now()
	b, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(sess.ID), b, 0o600)
}

// Load reads one session.
func (s *Store) Load(id string) (*Session, error) {
	b, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(b, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

// List returns sessions sorted newest-first.
func (s *Store) List() []*Session {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil
	}
	var out []*Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		if sess, err := s.Load(id); err == nil {
			out = append(out, sess)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

// Delete removes a session file.
func (s *Store) Delete(id string) error {
	return os.Remove(s.path(id))
}

// SetTitle updates the title (e.g. from the first user message).
func (sess *Session) SetTitleFromFirstUser() {
	for _, m := range sess.Messages {
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			t := strings.TrimSpace(m.Content)
			runes := []rune(t)
			if len(runes) > 60 {
				t = string(runes[:60]) + "…"
			}
			t = strings.ReplaceAll(t, "\n", " ")
			if sess.Title == "New session" || sess.Title == "" {
				sess.Title = t
			}
			return
		}
	}
}
