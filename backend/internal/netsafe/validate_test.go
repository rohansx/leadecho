package netsafe

import (
	"context"
	"testing"
)

func TestValidatePublicURL(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{"http://169.254.169.254/latest/meta-data/", true},
		{"http://127.0.0.1:8090/", true},
		{"http://localhost/", true},
		{"http://10.0.0.5/", true},
		{"http://[::1]/", true},
		{"ftp://example.com/", true},
		{"not a url", true},
		{"https://93.184.215.14/", false}, // literal public IP: no DNS needed
	}
	for _, tt := range tests {
		err := ValidatePublicURL(context.Background(), tt.url)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidatePublicURL(%q) err=%v, wantErr=%v", tt.url, err, tt.wantErr)
		}
	}
}
