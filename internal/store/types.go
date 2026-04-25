package store

import "time"

type Collection struct {
	Version   int                 `json:"version"`
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
	Requests  map[string]*Request `json:"requests"`
	Order     []string            `json:"order"`
}

type Request struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Method  string         `json:"method"`
	URL     string         `json:"url"`
	Headers []Header       `json:"headers"`
	Body    Body           `json:"body"`
	Auth    Auth           `json:"auth"`
	Tests   string         `json:"tests,omitempty"`
	Options RequestOptions `json:"options,omitempty"`
}

type RequestOptions struct {
	SkipTLSVerify    bool   `json:"skip_tls_verify,omitempty"`
	DisableRedirects bool   `json:"disable_redirects,omitempty"`
	ProxyURL         string `json:"proxy_url,omitempty"`
	TimeoutSecs      int    `json:"timeout_secs,omitempty"`
}

type Header struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

type Body struct {
	Mode string `json:"mode"` // raw, form
	Raw  string `json:"raw"`
}

type Config struct {
	Version     int    `json:"version"`
	TimeoutSecs int    `json:"timeout_secs"` // default 30
	Theme       string `json:"theme"`         // dark, light
}

type Auth struct {
	Type  string `json:"type"` // none, bearer, basic
	Token string `json:"token"`
	User  string `json:"user"`
	Pass  string `json:"pass"`
}

type Environment struct {
	Version    int      `json:"version"`
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Variables  []EnvVar `json:"variables"`
	DotenvPath string   `json:"dotenv_path,omitempty"`
}

type EnvVar struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
	Secret  bool   `json:"secret"`
}

type History struct {
	Version int            `json:"version"`
	Entries []HistoryEntry `json:"entries"`
}

type HistoryEntry struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Request   HistReq   `json:"request"`
	Response  HistResp  `json:"response"`
}

type HistReq struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type HistResp struct {
	StatusCode  int    `json:"status_code"`
	ElapsedMs   int64  `json:"elapsed_ms"`
	SizeBytes   int    `json:"size_bytes"`
	Body        string `json:"body,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

type AppState struct {
	Version            int    `json:"version"`
	ActiveCollectionID string `json:"active_collection_id"`
	ActiveEnvID        string `json:"active_env_id"`
	ActiveRequestID    string `json:"active_request_id"`
	SeenWelcome        bool   `json:"seen_welcome"`
}
