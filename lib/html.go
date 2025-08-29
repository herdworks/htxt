package hypertexting

type HTMLLink struct {
	As string `json:"as,omitempty" yaml:"as,omitempty"`
	Blocking string `json:"blocking,omitempty" yaml:"blocking,omitempty"`
	CrossOrigin string `json:"crossorigin,omitempty" yaml:"crossorigin,omitempty"`
	Disabled string `json:"disabled,omitempty" yaml:"disabled,omitempty"`
	FetchPriority string `json:"fetchpriority,omitempty" yaml:"fetchpriority,omitempty"`
	Href string `json:"href,omitempty" yaml:"href,omitempty"`
	HrefLang string `json:"hreflang,omitempty" yaml:"hreflang,omitempty"`
	ImageSizes string `json:"imagesizes,omitempty" yaml:"imagesizes,omitempty"`
	ImageSrcSet string `json:"imagesrcset,omitempty" yaml:"imagesrcset,omitempty"`
	Integrity string `json:"integrity,omitempty" yaml:"integrity,omitempty"`
	Media string `json:"media,omitempty" yaml:"media,omitempty"`
	ReferrerPolicy string `json:"referrerpolicy,omitempty" yaml:"referrerpolicy,omitempty"`
	Rel string `json:"rel,omitempty" yaml:"rel,omitempty"`
	Sizes string `json:"sizes,omitempty" yaml:"sizes,omitempty"`
	Target string `json:"target,omitempty" yaml:"target,omitempty"`
	Title string `json:"title,omitempty" yaml:"title,omitempty"`
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
}

type HTMLMeta struct {
	Charset string `json:"charset,omitempty" yaml:"charset,omitempty"`
	Content string `json:"content,omitempty" yaml:"content,omitempty"`
	HTTPEquiv string `json:"http-equiv,omitempty" yaml:"http-equiv,omitempty"`
	Media string `json:"media,omitempty" yaml:"media,omitempty"`
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	Property string `json:"property,omitempty" yaml:"property,omitempty"`
}
