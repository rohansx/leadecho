package researcher

import (
	"encoding/json"
	"strings"
	"testing"

	"leadecho/internal/database"
)

func TestPersonViewMetadataIsJSONObject(t *testing.T) {
	v := PersonView{Person: database.Person{ID: "p1", Metadata: []byte(`{"source":"mention_author"}`)}}
	v.Metadata = rawJSON(v.Person.Metadata)
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"metadata":{"source":"mention_author"}`) {
		t.Fatalf("metadata not an object: %s", b)
	}
	if got := rawJSON([]byte("not json")); string(got) != "{}" {
		t.Fatalf("invalid metadata should become {}, got %s", got)
	}
}
