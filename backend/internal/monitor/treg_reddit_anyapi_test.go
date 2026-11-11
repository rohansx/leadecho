package monitor

import (
	"testing"

	"leadecho/internal/database"
)

// Row shape captured live from anyapi.reddit.search (2026-11-11).
const anyapiRedditBody = `{"output":{"data":{"nextCursor":"cur_x","posts":[
 {"author":"em_lead","authorId":"7w3","createdUtc":1794196323,"id":"1xnrx05","nsfw":false,"numComments":37,
  "permalink":"/r/cscareerquestions/comments/1xnrx05/how_do_you_screen/","score":8,
  "selftext":"We keep hiring people who pass the coding round and then can't make decisions.",
  "subreddit":"cscareerquestions","title":"How do you screen experienced engineers for judgment?",
  "url":"https://www.reddit.com/r/cscareerquestions/comments/1xnrx05/how_do_you_screen/"},
 {"author":"linker","createdUtc":1794196000,"id":"1xabc","numComments":2,"permalink":"/r/startups/comments/1xabc/a_link/",
  "score":3,"selftext":"","subreddit":"startups","title":"A link post about hiring","url":"https://example.com/article"}
]}}}`

func TestParseAnyapiRedditRows(t *testing.T) {
	posts := parseTregPosts([]byte(anyapiRedditBody))
	if len(posts) != 2 {
		t.Fatalf("want 2 posts, got %d", len(posts))
	}
	p := posts[0]
	if p.ID != "1xnrx05" || p.Author != "em_lead" || p.Subreddit != "cscareerquestions" || p.Replies != 37 || p.Likes != 8 {
		t.Fatalf("row not mapped: %+v", p)
	}
	if p.Text != "We keep hiring people who pass the coding round and then can't make decisions." || p.CreatedAt.IsZero() {
		t.Fatalf("body/time not mapped: %+v", p)
	}
	// A link post has no body (title stands in) and must link to the thread, not the article.
	link := posts[1]
	if link.Text != "A link post about hiring" || link.URL != "https://www.reddit.com/r/startups/comments/1xabc/a_link/" {
		t.Fatalf("link post: %+v", link)
	}
	params, ok := tregRedditParams("ws", database.ListActiveKeywordsRow{ID: "", Term: "screen engineers"}, p)
	if !ok || params.PlatformID != "reddit_1xnrx05" || params.Title.String != "How do you screen experienced engineers for judgment?" {
		t.Fatalf("mention params: ok=%v %+v", ok, params)
	}
}
