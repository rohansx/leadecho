package competitors

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"sort"
	"strings"
	"time"
)

// Item is one new thing a source published: a post, a feed entry, a release,
// or a change to a page.
type Item struct {
	Key   string // stable per source; what makes it "not seen before"
	Title string
	Body  string
	URL   string
	At    time.Time // zero = unknown
}

const (
	maxFeedItems = 30
	maxItemBody  = 4000
)

type feedDoc struct {
	XMLName xml.Name
	Channel struct {
		Items []feedEntry `xml:"item"`
	} `xml:"channel"`
	Items   []feedEntry `xml:"item"`  // RSS 1.0 (RDF) puts items at the root
	Entries []feedEntry `xml:"entry"` // Atom
}

type feedEntry struct {
	Title string `xml:"title"`
	GUID  string `xml:"guid"`
	ID    string `xml:"id"`
	Links []struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
		Text string `xml:",chardata"`
	} `xml:"link"`
	Description string `xml:"description"`
	Encoded     string `xml:"encoded"`
	Summary     string `xml:"summary"`
	Content     string `xml:"content"`
	PubDate     string `xml:"pubDate"`
	Date        string `xml:"date"`
	Updated     string `xml:"updated"`
	Published   string `xml:"published"`
}

var feedTimeLayouts = []string{time.RFC1123Z, time.RFC1123, time.RFC3339, time.RFC822Z, time.RFC822, "2006-01-02T15:04:05", "2006-01-02"}

func parseFeedTime(vals ...string) time.Time {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		for _, l := range feedTimeLayouts {
			if t, err := time.Parse(l, v); err == nil {
				return t.UTC()
			}
		}
	}
	return time.Time{}
}

// ParseFeed reads an RSS or Atom document. Anything else is an error, so a
// caller can try a URL as a feed first and fall back to reading it as a page.
func ParseFeed(b []byte) ([]Item, error) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	var doc feedDoc
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	switch strings.ToLower(doc.XMLName.Local) {
	case "rss", "feed", "rdf":
	default:
		return nil, errors.New("not a feed")
	}
	entries := append(append(doc.Channel.Items, doc.Items...), doc.Entries...)
	items := make([]Item, 0, len(entries))
	for _, e := range entries {
		link := ""
		for _, l := range e.Links {
			href := strings.TrimSpace(l.Href)
			if href == "" {
				href = strings.TrimSpace(l.Text)
			}
			if href != "" && (link == "" || l.Rel == "alternate") {
				link = href
			}
		}
		key := firstNonEmpty(e.GUID, e.ID, link)
		if key == "" {
			continue
		}
		body := firstNonEmpty(e.Encoded, e.Content, e.Description, e.Summary)
		items = append(items, Item{
			Key:   clip(key, 500),
			Title: clip(strings.Join(strings.Fields(e.Title), " "), 300),
			Body:  clip(strings.Join(strings.Fields(PageText(body)), " "), maxItemBody),
			URL:   link,
			At:    parseFeedTime(e.PubDate, e.Published, e.Updated, e.Date),
		})
	}
	// Some feeds list oldest first: keep the newest ones, not the first ones.
	dated := true
	for _, it := range items {
		dated = dated && !it.At.IsZero()
	}
	if dated {
		sort.SliceStable(items, func(i, j int) bool { return items[i].At.After(items[j].At) })
	}
	if len(items) > maxFeedItems {
		items = items[:maxFeedItems]
	}
	return items, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// clip cuts s to at most n runes.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
