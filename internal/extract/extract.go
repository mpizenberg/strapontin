package extract

import (
	"bytes"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Version is stored with every snapshot; bump it whenever extraction changes,
// so stored pages can be re-extracted.
const Version = 2

// Credit is a name found on the page, with its role key.
type Credit struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Character string `json:"character,omitempty"`
}

// Draft is what a page says about a production, before any matching.
type Draft struct {
	Title        string   `json:"title"`
	WorkTitle    string   `json:"work_title,omitempty"`
	Company      string   `json:"company,omitempty"`
	Description  string   `json:"description,omitempty"`
	Image        string   `json:"image,omitempty"`
	VenueName    string   `json:"venue_name,omitempty"`
	VenueCity    string   `json:"venue_city,omitempty"`
	VenueAddress string   `json:"venue_address,omitempty"`
	StartsOn     string   `json:"starts_on,omitempty"`
	EndsOn       string   `json:"ends_on,omitempty"`
	Credits      []Credit `json:"credits"`
	// Source is the richest structured source found: jsonld, microdata, opengraph or html.
	Source string `json:"source"`
	// Text is the visible text of the page, for the credits parser.
	Text string `json:"-"`
}

// set fills a field only if it is still empty: sources are applied from the
// most to the least reliable.
func set(dst *string, v string) {
	if *dst == "" {
		*dst = clean(v)
	}
}

// clean decodes leftover entities (some sites double-encode them) and collapses whitespace.
func clean(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
}

// toUTF8 decodes Latin-1 pages; anything else is assumed to be UTF-8.
func toUTF8(body []byte, contentType string) []byte {
	ct := strings.ToLower(contentType)
	if utf8.Valid(body) && !strings.Contains(ct, "8859-1") && !strings.Contains(ct, "1252") {
		return body
	}
	var b bytes.Buffer
	for _, c := range body {
		b.WriteRune(rune(c))
	}
	return b.Bytes()
}

// Extract builds a draft from a page body.
func Extract(body []byte, contentType, pageURL string) (*Draft, error) {
	doc, err := html.Parse(bytes.NewReader(toUTF8(body, contentType)))
	if err != nil {
		return nil, err
	}
	p := scan(doc)
	d := &Draft{Credits: []Credit{}}

	if applyJSONLD(d, p.jsonld) {
		d.Source = "jsonld"
	}
	if applyMicrodata(d, p.microdata) && d.Source == "" {
		d.Source = "microdata"
	}
	if p.meta["og:title"] != "" && d.Source == "" {
		d.Source = "opengraph"
	}
	set(&d.Title, trimSiteName(p.meta["og:title"], p.meta["og:site_name"]))
	set(&d.Description, p.meta["og:description"])
	set(&d.Image, p.meta["og:image"])
	set(&d.Title, trimSiteName(p.title, p.meta["og:site_name"]))
	set(&d.Description, p.meta["description"])
	if d.Source == "" {
		d.Source = "html"
	}

	d.Image = absolute(pageURL, d.Image)
	d.Text = p.text.String()

	// Structured data gives the when and where; the text gives the who.
	text := d.Text
	if !strings.Contains(text, d.Description) {
		text += "\n" + d.Description
	}
	credits, company := ParseCredits(text)
	for _, c := range credits {
		addCredit(d, c)
	}
	set(&d.Company, company)
	return d, nil
}

// trimSiteName removes a trailing " | Site" or " - Site" from a page title.
func trimSiteName(title, site string) string {
	title = clean(title)
	for _, sep := range []string{" | ", " – ", " — ", " - "} {
		if i := strings.LastIndex(title, sep); i > 0 {
			tail := title[i+len(sep):]
			if site == "" || strings.EqualFold(tail, clean(site)) || sep == " | " {
				return strings.TrimSpace(title[:i])
			}
		}
	}
	return title
}

func absolute(base, ref string) string {
	if ref == "" {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := b.Parse(ref)
	if err != nil || (r.Scheme != "http" && r.Scheme != "https") {
		return ""
	}
	return r.String()
}

// page holds what one walk over the document collects.
type page struct {
	title     string
	meta      map[string]string // og:* properties and name=description
	jsonld    []string
	microdata []item
	text      strings.Builder
	last      byte // last byte written to text
}

func (p *page) write(s string) {
	if s == "\n" && (p.last == '\n' || p.text.Len() == 0) {
		return
	}
	if s != "\n" && p.last != '\n' && p.last != ' ' && p.text.Len() > 0 {
		// Keep a separating space between inline elements.
		p.text.WriteByte(' ')
	}
	p.text.WriteString(s)
	p.last = s[len(s)-1]
}

var skipText = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Template: true,
	atom.Svg: true, atom.Iframe: true, atom.Nav: true, atom.Footer: true, atom.Form: true, atom.Head: true,
}

var blocks = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Br: true, atom.Li: true, atom.H1: true, atom.H2: true, atom.H3: true,
	atom.H4: true, atom.H5: true, atom.H6: true, atom.Tr: true, atom.Dt: true, atom.Dd: true, atom.Section: true,
	atom.Article: true, atom.Header: true, atom.Ul: true, atom.Ol: true, atom.Table: true, atom.Blockquote: true,
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func scan(doc *html.Node) *page {
	p := &page{meta: map[string]string{}}
	var walk func(n *html.Node, inText bool)
	walk = func(n *html.Node, inText bool) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Title:
				if p.title == "" && n.FirstChild != nil {
					p.title = n.FirstChild.Data
				}
			case atom.Meta:
				key := attr(n, "property")
				if key == "" {
					key = attr(n, "name")
				}
				key = strings.ToLower(key)
				if (strings.HasPrefix(key, "og:") || key == "description") && p.meta[key] == "" {
					p.meta[key] = attr(n, "content")
				}
			case atom.Script:
				if strings.Contains(strings.ToLower(attr(n, "type")), "ld+json") && n.FirstChild != nil {
					p.jsonld = append(p.jsonld, n.FirstChild.Data)
				}
			}
			if _, ok := hasAttr(n, "itemscope"); ok && attr(n, "itemprop") == "" {
				p.microdata = append(p.microdata, readItem(n))
			}
			if skipText[n.DataAtom] {
				inText = false
			}
			if blocks[n.DataAtom] && inText {
				p.write("\n")
			}
		}
		if n.Type == html.TextNode && inText {
			if t := strings.Join(strings.Fields(n.Data), " "); t != "" {
				p.write(t)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inText)
		}
		if n.Type == html.ElementNode && blocks[n.DataAtom] && inText {
			p.write("\n")
		}
	}
	walk(doc, true)
	return p
}

func hasAttr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

// textContent returns the text inside a node, whitespace collapsed.
func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}
