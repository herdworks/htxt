package href

import (
	"encoding/json"
	"net/url"
)

// Href wraps url.URL with string-based JSON marshaling.
type Href struct {
	url.URL
}

func Parse(raw string) (*Href, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &Href{*u}, nil
}

func (u Href) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

func (u *Href) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	u.URL = *parsed
	return nil
}

func (u Href) MarshalText() ([]byte, error) {
	return []byte(u.String()), nil
}

func (u *Href) UnmarshalText(data []byte) error {
	parsed, err := url.Parse(string(data))
	if err != nil {
		return err
	}
	u.URL = *parsed
	return nil
}
