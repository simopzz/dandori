package domain

// ContentLanguage is the one language an installation writes every food
// term in, as an ISO 639-1 code.
type ContentLanguage string

const (
	Italian ContentLanguage = "it"
	English ContentLanguage = "en"
)

// ContentLanguages lists every supported ContentLanguage.
var ContentLanguages = []ContentLanguage{Italian, English}

// Name is the language's English name, for prompts and messages.
func (l ContentLanguage) Name() string {
	switch l {
	case Italian:
		return "Italian"
	case English:
		return "English"
	}
	return string(l)
}
