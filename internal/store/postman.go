package store

import (
	"encoding/json"
	"fmt"
	"time"
)

// Postman collection v2.1 structures (minimal, covers the common fields).

type pmCollection struct {
	Info pmInfo   `json:"info"`
	Item []pmItem `json:"item"`
}

type pmInfo struct {
	Name   string `json:"name"`
	Schema string `json:"schema"`
}

type pmItem struct {
	Name    string    `json:"name"`
	Item    []pmItem  `json:"item,omitempty"`    // folder
	Request *pmReq    `json:"request,omitempty"` // leaf request
}

type pmReq struct {
	Method string     `json:"method"`
	URL    pmURL      `json:"url"`
	Header []pmHeader `json:"header,omitempty"`
	Body   *pmBody    `json:"body,omitempty"`
	Auth   *pmAuth    `json:"auth,omitempty"`
}

type pmURL struct {
	Raw string `json:"raw"`
}

type pmHeader struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled"`
}

type pmBody struct {
	Mode string `json:"mode"`
	Raw  string `json:"raw"`
}

type pmAuth struct {
	Type   string     `json:"type"`
	Bearer []pmKeyVal `json:"bearer,omitempty"`
	Basic  []pmKeyVal `json:"basic,omitempty"`
}

type pmKeyVal struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ImportPostman parses a Postman v2.1 collection JSON and returns a Collection.
// Folders are flattened - all requests end up at the top level.
func ImportPostman(data []byte) (*Collection, error) {
	var pm pmCollection
	if err := json.Unmarshal(data, &pm); err != nil {
		return nil, fmt.Errorf("parse postman: %w", err)
	}

	c := &Collection{
		Version:   1,
		ID:        newID(),
		Name:      pm.Info.Name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Requests:  make(map[string]*Request),
	}

	var walk func(items []pmItem)
	walk = func(items []pmItem) {
		for _, item := range items {
			if len(item.Item) > 0 {
				walk(item.Item) // folder
				continue
			}
			if item.Request == nil {
				continue
			}
			r := convertPMRequest(item.Name, item.Request)
			c.Requests[r.ID] = r
			c.Order = append(c.Order, r.ID)
		}
	}
	walk(pm.Item)

	return c, nil
}

// ExportPostman serialises a Collection to Postman v2.1 JSON.
func ExportPostman(c *Collection) ([]byte, error) {
	pm := pmCollection{
		Info: pmInfo{
			Name:   c.Name,
			Schema: "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
		},
	}

	for _, id := range c.Order {
		r, ok := c.Requests[id]
		if !ok {
			continue
		}
		pm.Item = append(pm.Item, pmItem{
			Name:    r.Name,
			Request: convertStorageRequest(r),
		})
	}
	// requests not in Order
	inOrder := make(map[string]bool, len(c.Order))
	for _, id := range c.Order {
		inOrder[id] = true
	}
	for _, r := range c.Requests {
		if !inOrder[r.ID] {
			pm.Item = append(pm.Item, pmItem{
				Name:    r.Name,
				Request: convertStorageRequest(r),
			})
		}
	}

	return json.MarshalIndent(pm, "", "  ")
}

func convertPMRequest(name string, req *pmReq) *Request {
	r := &Request{
		ID:     newID(),
		Name:   name,
		Method: req.Method,
		URL:    req.URL.Raw,
		Body:   Body{Mode: "raw"},
	}
	for _, h := range req.Header {
		r.Headers = append(r.Headers, Header{
			Key:     h.Key,
			Value:   h.Value,
			Enabled: !h.Disabled,
		})
	}
	if req.Body != nil {
		r.Body = Body{Mode: req.Body.Mode, Raw: req.Body.Raw}
	}
	if req.Auth != nil {
		switch req.Auth.Type {
		case "bearer":
			token := kvGet(req.Auth.Bearer, "token")
			r.Auth = Auth{Type: "bearer", Token: token}
		case "basic":
			r.Auth = Auth{
				Type: "basic",
				User: kvGet(req.Auth.Basic, "username"),
				Pass: kvGet(req.Auth.Basic, "password"),
			}
		}
	}
	return r
}

func convertStorageRequest(r *Request) *pmReq {
	req := &pmReq{
		Method: r.Method,
		URL:    pmURL{Raw: r.URL},
	}
	for _, h := range r.Headers {
		req.Header = append(req.Header, pmHeader{
			Key:      h.Key,
			Value:    h.Value,
			Disabled: !h.Enabled,
		})
	}
	if r.Body.Raw != "" {
		req.Body = &pmBody{Mode: r.Body.Mode, Raw: r.Body.Raw}
	}
	switch r.Auth.Type {
	case "bearer":
		req.Auth = &pmAuth{
			Type:   "bearer",
			Bearer: []pmKeyVal{{Key: "token", Value: r.Auth.Token}},
		}
	case "basic":
		req.Auth = &pmAuth{
			Type: "basic",
			Basic: []pmKeyVal{
				{Key: "username", Value: r.Auth.User},
				{Key: "password", Value: r.Auth.Pass},
			},
		}
	}
	return req
}

func kvGet(pairs []pmKeyVal, key string) string {
	for _, p := range pairs {
		if p.Key == key {
			return p.Value
		}
	}
	return ""
}
