package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Store struct {
	dir string
}

func New() (*Store, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	return NewAt(dir)
}

// NewAt creates a Store rooted at dir, creating required sub-directories.
// Use this in tests to point the store at a temporary directory.
func NewAt(dir string) (*Store, error) {
	for _, sub := range []string{"collections", "environments", "plugins", "themes"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("create %s dir: %w", sub, err)
		}
	}
	return &Store{dir: dir}, nil
}

// Dir returns the root config directory (e.g. ~/.config/gopull).
func (s *Store) Dir() string { return s.dir }

func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}
	return filepath.Join(base, "gopull"), nil
}

func (s *Store) LoadCollections() ([]*Collection, error) {
	dir := filepath.Join(s.dir, "collections")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read collections: %w", err)
	}
	var cols []*Collection
	var loadErrs []error
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		c, err := s.loadCollection(filepath.Join(dir, e.Name()))
		if err != nil {
			loadErrs = append(loadErrs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		cols = append(cols, c)
	}
	sort.Slice(cols, func(i, j int) bool { return cols[i].Name < cols[j].Name })
	if len(cols) == 0 && len(loadErrs) > 0 {
		return nil, loadErrs[0]
	}
	return cols, nil
}

func (s *Store) loadCollection(path string) (*Collection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Collection
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.Requests == nil {
		c.Requests = make(map[string]*Request)
	}
	return &c, nil
}

func (s *Store) SaveCollection(c *Collection) error {
	if c.ID == "" {
		c.ID = newID()
		c.CreatedAt = time.Now()
	}
	c.UpdatedAt = time.Now()
	c.Version = 1
	path := filepath.Join(s.dir, "collections", c.ID+".json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal collection: %w", err)
	}
	return writeAtomic(path, data)
}

func (s *Store) EnsureDefaultCollection() (*Collection, error) {
	cols, err := s.LoadCollections()
	if err != nil {
		return nil, err
	}
	if len(cols) > 0 {
		return cols[0], nil
	}
	c := &Collection{Name: "Default", Requests: make(map[string]*Request)}
	return c, s.SaveCollection(c)
}

func (s *Store) DeleteRequest(collectionID, requestID string) error {
	path := filepath.Join(s.dir, "collections", collectionID+".json")
	c, err := s.loadCollection(path)
	if err != nil {
		return fmt.Errorf("load collection: %w", err)
	}
	delete(c.Requests, requestID)
	newOrder := make([]string, 0, len(c.Order))
	for _, id := range c.Order {
		if id != requestID {
			newOrder = append(newOrder, id)
		}
	}
	c.Order = newOrder
	return s.SaveCollection(c)
}

func (s *Store) SaveRequest(collectionID string, r *Request) error {
	path := filepath.Join(s.dir, "collections", collectionID+".json")
	c, err := s.loadCollection(path)
	if err != nil {
		return fmt.Errorf("load collection: %w", err)
	}
	if r.ID == "" {
		r.ID = newID()
		c.Order = append(c.Order, r.ID)
	}
	if c.Requests == nil {
		c.Requests = make(map[string]*Request)
	}
	c.Requests[r.ID] = r
	return s.SaveCollection(c)
}

func (s *Store) LoadEnvironments() ([]*Environment, error) {
	dir := filepath.Join(s.dir, "environments")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read environments: %w", err)
	}
	var envs []*Environment
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		env, err := s.loadEnvironment(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		envs = append(envs, env)
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })
	return envs, nil
}

func (s *Store) loadEnvironment(path string) (*Environment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var e Environment
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Store) SaveEnvironment(e *Environment) error {
	if e.ID == "" {
		e.ID = newID()
	}
	e.Version = 1
	path := filepath.Join(s.dir, "environments", e.ID+".json")
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal environment: %w", err)
	}
	return writeAtomic(path, data)
}

func (s *Store) AppendHistory(entry HistoryEntry) error {
	h, err := s.LoadHistory()
	if err != nil {
		return fmt.Errorf("load history before append: %w", err)
	}
	if h == nil {
		h = &History{Version: 1}
	}
	if entry.ID == "" {
		entry.ID = newID()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	h.Entries = append([]HistoryEntry{entry}, h.Entries...)
	if len(h.Entries) > 500 {
		h.Entries = h.Entries[:500]
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal history: %w", err)
	}
	return writeAtomic(filepath.Join(s.dir, "history.json"), data)
}

func (s *Store) LoadHistory() (*History, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, "history.json"))
	if os.IsNotExist(err) {
		return &History{Version: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	var h History
	if err := json.Unmarshal(data, &h); err != nil {
		return &History{Version: 1}, nil
	}
	return &h, nil
}

func (s *Store) LoadState() (*AppState, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, "state.json"))
	if os.IsNotExist(err) {
		return &AppState{Version: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	var st AppState
	if err := json.Unmarshal(data, &st); err != nil {
		return &AppState{Version: 1}, nil
	}
	return &st, nil
}

func (s *Store) SaveState(st *AppState) error {
	st.Version = 1
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return writeAtomic(filepath.Join(s.dir, "state.json"), data)
}

func (s *Store) DeleteCollection(collID string) error {
	path := filepath.Join(s.dir, "collections", collID+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete collection: %w", err)
	}
	return nil
}

func (s *Store) LoadConfig() (*Config, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, "config.json"))
	if os.IsNotExist(err) {
		return &Config{TimeoutSecs: 30}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return &Config{TimeoutSecs: 30}, nil
	}
	if c.TimeoutSecs <= 0 {
		c.TimeoutSecs = 30
	}
	return &c, nil
}

func (s *Store) SaveConfig(c *Config) error {
	c.Version = 1
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return writeAtomic(filepath.Join(s.dir, "config.json"), data)
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func (s *Store) LoadKeybindings() (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, "keybindings.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var kb map[string]string
	return kb, json.Unmarshal(data, &kb)
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
