package extract

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// labels maps the French labels found before names to role keys. An empty
// role marks a label that ends the previous list without starting a new one.
var labels = map[string]string{
	"texte": "author", "textes": "author", "texte original": "author", "écriture": "author",
	"auteur": "author", "autrice": "author", "auteurs": "author", "autrices": "author", "d'après": "author", "d’après": "author",
	"adaptation": "adaptation", "traduction": "translation",
	"mise en scène": "director", "mise en piste": "director", "conception et mise en scène": "director",
	"conception": "director", "écriture et mise en scène": "director", "texte et mise en scène": "director",
	"assistanat à la mise en scène": "assistant_director", "assistant à la mise en scène": "assistant_director",
	"assistante à la mise en scène": "assistant_director", "assistanat mise en scène": "assistant_director",
	"assistante mise en scène": "assistant_director", "assistant mise en scène": "assistant_director",
	"avec": "performer", "interprétation": "performer", "jeu": "performer", "distribution": "performer",
	"interprètes": "performer", "comédiens": "performer", "comédiennes": "performer",
	"dramaturgie": "dramaturgy",
	"scénographie": "set_design", "décor": "set_design", "décors": "set_design",
	"lumière": "lighting", "lumières": "lighting", "création lumière": "lighting", "création lumières": "lighting", "éclairages": "lighting",
	"son": "sound", "création sonore": "sound", "création son": "sound", "régie son": "",
	"musique": "music", "composition": "music", "création musicale": "music", "composition musicale": "music",
	"costumes": "costumes", "costume": "costumes",
	"chorégraphie": "choreography",
	"vidéo": "video", "création vidéo": "video",
	"collaboration artistique": "", "production": "", "coproduction": "", "diffusion": "", "administration": "",
	"régie": "", "régie générale": "", "durée": "", "tarif": "", "tarifs": "", "photo": "", "photos": "",
	"crédit photo": "", "remerciements": "", "avec le soutien": "", "soutien": "", "construction": "",
}

var labelRe = func() *regexp.Regexp {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, regexp.QuoteMeta(k))
	}
	// Longest first, so "mise en scène" wins over "mise en piste" prefixes and "texte original" over "texte".
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	return regexp.MustCompile(`(?i)(` + strings.Join(keys, "|") + `)(\s*:\s*|\s+(?:de|du|par|d'|d’)\s*|\s+|$)`)
}()

// separators split a list of names.
var separators = regexp.MustCompile(`\s*(?:,|;|/|&|\set\s|\sand\s|·|•)\s*`)

// characterRe reads "Name (Character)" and "Name : Character" in casts.
var characterRe = regexp.MustCompile(`^(.+?)\s*\((.+)\)$`)

// groupWords mark a company rather than a person.
var groupWords = map[string]bool{"compagnie": true, "cie": true, "collectif": true, "ensemble": true, "troupe": true, "théâtre": true, "orchestre": true}

// particles are the lowercase words allowed inside a name.
var particles = map[string]bool{"de": true, "du": true, "des": true, "d'": true, "d’": true, "le": true, "la": true, "van": true, "von": true, "da": true, "di": true, "del": true, "ben": true, "el": true, "al": true, "y": true}

// ParseCredits reads labelled lists of names from free text, such as the
// credits block of a show page or text pasted by a member.
func ParseCredits(text string) (credits []Credit, company string) {
	role := "" // the label of a line that held nothing else carries to the next line
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		matches := labelMatches(line)
		if len(matches) == 0 {
			if role != "" {
				credits, company = addNames(credits, company, role, line)
			}
			role = ""
			continue
		}
		role = ""
		for i, m := range matches {
			end := len(line)
			if i+1 < len(matches) {
				end = matches[i+1][0]
			}
			segment := strings.TrimSpace(line[m[1]:end])
			r := labels[strings.ToLower(line[m[2]:m[3]])]
			if segment == "" && i == len(matches)-1 {
				role = r
				continue
			}
			if r != "" {
				credits, company = addNames(credits, company, r, segment)
			}
		}
	}
	return credits, company
}

// labelMatches finds labels that stand as whole words and are followed by
// something that can start a name, so "son père" or "avec humour" don't count.
func labelMatches(line string) [][]int {
	var out [][]int
	for _, m := range labelRe.FindAllStringSubmatchIndex(line, -1) {
		if m[0] > 0 {
			if r, _ := utf8.DecodeLastRuneInString(line[:m[0]]); unicode.IsLetter(r) {
				continue
			}
		}
		rest := line[m[1]:]
		next, _ := utf8.DecodeRuneInString(rest)
		colon := strings.Contains(line[m[4]:m[5]], ":")
		if rest != "" && !colon {
			if unicode.IsLetter(rune(line[m[1]-1])) {
				continue // label glued to the next word: "Sonate"
			}
			if !unicode.IsUpper(next) {
				continue
			}
		}
		out = append(out, m)
	}
	return out
}

func addNames(credits []Credit, company, role, segment string) ([]Credit, string) {
	// A sentence after the names ends the list.
	if i := strings.Index(segment, ". "); i > 0 {
		segment = segment[:i]
	}
	for _, raw := range separators.Split(strings.TrimRight(segment, ". "), 30) {
		name, character := strings.TrimSpace(raw), ""
		if m := characterRe.FindStringSubmatch(name); m != nil && role == "performer" {
			name, character = strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
		}
		switch kind(name) {
		case "person":
			if !hasCredit(credits, role, name) {
				credits = append(credits, Credit{Name: name, Role: role, Character: character})
			}
		case "group":
			if company == "" {
				company = name
			}
		}
	}
	return credits, company
}

func hasCredit(credits []Credit, role, name string) bool {
	for _, c := range credits {
		if c.Role == role && strings.EqualFold(c.Name, name) {
			return true
		}
	}
	return false
}

// kind tells a person's name from a company and from anything else.
func kind(name string) string {
	words := strings.Fields(name)
	if len(words) == 0 || len(words) > 6 || len(name) > 60 || strings.ContainsAny(name, "0123456789@:") {
		return ""
	}
	if groupWords[strings.ToLower(strings.Trim(words[0], ".'’"))] {
		return "group"
	}
	capitals := 0
	for _, w := range words {
		first, _ := utf8.DecodeRuneInString(w)
		switch {
		case unicode.IsUpper(first):
			capitals++
		case particles[strings.ToLower(w)] || strings.HasPrefix(w, "d'") || strings.HasPrefix(w, "d’"):
		default:
			return ""
		}
	}
	if capitals == 0 || (len(words) == 1 && utf8.RuneCountInString(name) < 3) {
		return ""
	}
	return "person"
}
