package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Footer is enabled by default; JSON false disables the entire footer.
type Footer struct {
	Disabled bool          `json:"-"`
	Wordmark string        `json:"wordmark"`
	Tagline  string        `json:"tagline"`
	Accent   string        `json:"accent"`
	Type     string        `json:"type"`
	Muted    string        `json:"muted"`
	Groups   []FooterGroup `json:"groups"`
	Badges   []FooterBadge `json:"badges"`
}

type FooterGroup struct {
	Label string       `json:"label"`
	Links []FooterLink `json:"links"`
}

type FooterLink struct {
	URLKey string `json:"urlKey"`
	Text   string `json:"text"`
	Lang   string `json:"lang"`
}

type FooterBadge struct {
	URLKey string `json:"urlKey"`
	SrcKey string `json:"srcKey"`
	Alt    string `json:"alt"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Target string `json:"target"`
	Rel    string `json:"rel"`
}

func (f *Footer) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("false")) {
		*f = Footer{Disabled: true}
		return nil
	}
	if len(raw) == 0 || raw[0] != '{' {
		return fmt.Errorf("footer must be false or an object")
	}
	type object Footer
	var value object
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	*f = Footer(value)
	return nil
}

func (f Footer) MarshalJSON() ([]byte, error) {
	if f.Disabled {
		return []byte("false"), nil
	}
	type object Footer
	return json.Marshal(object(f))
}

func (f Footer) validate(urls map[string]string, add func(string, string, error)) {
	key := func(path, value string) {
		if strings.TrimSpace(urls[value]) == "" {
			add(ConfigFile, path, fmt.Errorf("%s must name an existing urls key; see 'lfm help config footer'", path))
		}
	}
	for i, group := range f.Groups {
		for j, link := range group.Links {
			key(fmt.Sprintf("footer.groups[%d].links[%d].urlKey", i, j), link.URLKey)
		}
	}
	for i, badge := range f.Badges {
		path := fmt.Sprintf("footer.badges[%d].", i)
		key(path+"urlKey", badge.URLKey)
		key(path+"srcKey", badge.SrcKey)
		for _, size := range []struct {
			name  string
			value int
		}{{"width", badge.Width}, {"height", badge.Height}} {
			if size.value <= 0 {
				add(ConfigFile, path+size.name, fmt.Errorf("%s%s must be positive; see 'lfm help config footer.badges'", path, size.name))
			}
		}
	}
	for _, value := range []struct{ name, text string }{{"accent", f.Accent}, {"type", f.Type}, {"muted", f.Muted}} {
		if strings.ContainsAny(value.text, ";{}<>") {
			add(ConfigFile, "footer."+value.name, fmt.Errorf("footer.%s must be a single CSS value; see 'lfm help config footer'", value.name))
		}
	}
}
