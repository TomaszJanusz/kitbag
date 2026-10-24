package manifest

import "golang.org/x/text/language"

// PreferredLocale parses an Accept-Language header and returns the best tag.
// Throwaway: exercises golang.org/x/text v0.3.0 (GO-2021-0113, GO-2022-1059).
func PreferredLocale(header string) (language.Tag, error) {
	tags, _, err := language.ParseAcceptLanguage(header)
	if err != nil {
		return language.Und, err
	}
	if len(tags) == 0 {
		return language.Und, nil
	}
	return tags[0], nil
}
