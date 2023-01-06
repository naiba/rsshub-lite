package model

type Source struct {
	Name          string `json:"name,omitempty"`
	Slug          string `json:"slug,omitempty"`
	Url           string `json:"url,omitempty"`
	NextPageMatch string `json:"next_page_match,omitempty"`
	Delay         int    `json:"delay,omitempty"`

	ItemSelector        string `json:"item_selector,omitempty"`
	TitleSelector       string `json:"title_selector,omitempty"`
	DescriptionSelector string `json:"description_selector,omitempty"`
	LinkSelector        string `json:"link_selector,omitempty"`
}
