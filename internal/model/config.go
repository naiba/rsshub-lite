package model

type Config struct {
	Sources  []Source `json:"sources,omitempty"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	MaxItems int      `json:"max_items,omitempty"`
}
