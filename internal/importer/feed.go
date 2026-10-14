package importer

import (
	"context"
	"encoding/xml"
	"errors"
	"html"
	"regexp"
	"strings"
	"time"
)

// Item is one entry of a news feed (RSS 2.0 or Atom), for local sources (81).
type Item struct {
	Title string    `json:"title"`
	Link  string    `json:"link"`
	Text  string    `json:"text,omitempty"` // the summary as plain text, clipped
	At    time.Time `json:"at"`
	GUID  string    `json:"guid"` // stable across pulls: for dedupe
}

// Feed is a parsed news feed.
type Feed struct {
	Title string
	Link  string
	Lang  string // what the feed says it's in ("pt", from "pt-PT"), if it says
	Items []Item
}

// MaxItems bounds what one feed hands over per pull.
const MaxItems = 30

type rssDoc struct {
	XMLName xml.Name
	// RSS 2.0 (and RSS 1.0/RDF, whose items sit next to the channel)
	Channel struct {
		Title string    `xml:"title"`
		Link  string    `xml:"link"`
		Lang  string    `xml:"language"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
	XMLLang  string    `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	RDFItems []rssItem `xml:"item"`
	// Atom
	Title   string      `xml:"title"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	Desc    string `xml:"description"`
	PubDate string `xml:"pubDate"`
	Date    string `xml:"http://purl.org/dc/elements/1.1/ date"`
	GUID    string `xml:"guid"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	Links     []atomLink `xml:"link"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Updated   string     `xml:"updated"`
	Published string     `xml:"published"`
	ID        string     `xml:"id"`
}

// ParseFeed reads RSS 2.0, RSS 1.0 or Atom. ok is false when body is none
// of those. Items without a title and a web link are dropped; at most
// MaxItems, newest first as the feed lists them.
func ParseFeed(body string) (f Feed, ok bool) {
	var d rssDoc
	dec := xml.NewDecoder(strings.NewReader(body))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	if err := dec.Decode(&d); err != nil {
		return f, false
	}
	switch strings.ToLower(d.XMLName.Local) {
	case "rss", "rdf":
		f.Title, f.Link, f.Lang = clean(d.Channel.Title), strings.TrimSpace(d.Channel.Link), lang2(d.Channel.Lang)
		for _, it := range append(d.Channel.Items, d.RDFItems...) {
			at := parseDate(it.PubDate)
			if at.IsZero() {
				at = parseDate(it.Date)
			}
			f.add(Item{Title: clean(it.Title), Link: strings.TrimSpace(it.Link), Text: plain(it.Desc), At: at, GUID: strings.TrimSpace(it.GUID)})
		}
	case "feed":
		f.Title, f.Link, f.Lang = clean(d.Title), atomHref(d.Links), lang2(d.XMLLang)
		for _, e := range d.Entries {
			at := parseDate(e.Published)
			if at.IsZero() {
				at = parseDate(e.Updated)
			}
			sum := e.Summary
			if sum == "" {
				sum = e.Content
			}
			f.add(Item{Title: clean(e.Title), Link: atomHref(e.Links), Text: plain(sum), At: at, GUID: strings.TrimSpace(e.ID)})
		}
	default:
		return f, false
	}
	return f, true
}

func (f *Feed) add(it Item) {
	if len(f.Items) >= MaxItems || it.Title == "" || !(strings.HasPrefix(it.Link, "https://") || strings.HasPrefix(it.Link, "http://")) {
		return
	}
	if it.GUID == "" {
		it.GUID = it.Link
	}
	f.Items = append(f.Items, it)
}

func atomHref(ls []atomLink) string {
	for _, l := range ls {
		if l.Rel == "" || l.Rel == "alternate" {
			return strings.TrimSpace(l.Href)
		}
	}
	return ""
}

var tagRE = regexp.MustCompile(`(?s)<[^>]*>`)

// clean is a title as plain text on one line.
func clean(s string) string { return strings.Join(strings.Fields(plain(s)), " ") }

// plain strips markup and entities from a summary, clipped to 300 runes.
func plain(s string) string {
	s = strings.Join(strings.Fields(html.UnescapeString(tagRE.ReplaceAllString(html.UnescapeString(s), " "))), " ")
	if r := []rune(s); len(r) > 300 {
		s = string(r[:299]) + "…"
	}
	return s
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST",
		"2 Jan 2006 15:04:05 -0700", time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ErrNoFeed: the page at a source link is no RSS or Atom feed.
var ErrNoFeed = errors.New("importer: not an RSS or Atom feed")

// FetchFeed reads the news feed at raw, through the same guarded client
// as event pages (public addresses only, redirects checked, 3 MB cap).
func (im *Importer) FetchFeed(ctx context.Context, raw string) (Feed, error) {
	body, err := im.Get(ctx, raw)
	if err != nil {
		return Feed{}, err
	}
	f, ok := ParseFeed(body)
	if !ok {
		return Feed{}, ErrNoFeed
	}
	return f, nil
}

// lang2: "pt-PT", "en_us" → "pt", "en"; anything else → "".
func lang2(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) >= 2 && s[0] >= 'a' && s[0] <= 'z' && s[1] >= 'a' && s[1] <= 'z' && (len(s) == 2 || s[2] == '-' || s[2] == '_') {
		return s[:2]
	}
	return ""
}
