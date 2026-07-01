package cli

import "testing"

func TestBrowserOpenArgs(t *testing.T) {
	const target = "https://example.com/checkout"
	tests := []struct {
		goos     string
		wantName string
		wantArgs []string
		wantErr  bool
	}{
		{"darwin", "open", []string{target}, false},
		{"linux", "xdg-open", []string{target}, false},
		{"windows", "rundll32", []string{"url.dll,FileProtocolHandler", target}, false},
		{"plan9", "", nil, true},
	}
	for _, tt := range tests {
		name, gotArgs, err := browserOpenArgs(tt.goos, target)
		if tt.wantErr {
			if err == nil {
				t.Errorf("%s: expected error", tt.goos)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tt.goos, err)
			continue
		}
		if name != tt.wantName {
			t.Errorf("%s: name = %q, want %q", tt.goos, name, tt.wantName)
		}
		if len(gotArgs) != len(tt.wantArgs) {
			t.Errorf("%s: args = %v, want %v", tt.goos, gotArgs, tt.wantArgs)
			continue
		}
		for i := range gotArgs {
			if gotArgs[i] != tt.wantArgs[i] {
				t.Errorf("%s: args[%d] = %q, want %q", tt.goos, i, gotArgs[i], tt.wantArgs[i])
			}
		}
	}
}
