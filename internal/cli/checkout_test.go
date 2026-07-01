package cli

import (
	"context"
	"errors"
	"testing"
)

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

func TestCheckoutOpen_JSONStillOpensBrowser(t *testing.T) {
	oldExec := execStart
	defer func() { execStart = oldExec }()
	var openedName string
	var openedArgs []string
	execStart = func(name string, argv []string) error {
		openedName = name
		openedArgs = argv
		return nil
	}

	rt := runtime{flags: &Flags{JSON: true}, json: true}
	ctx := ctxWithRuntime(context.Background(), rt)
	cmd := newCheckoutOpenCmd()
	cmd.SetContext(ctx)

	out := captureStdout(t, func() {
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("checkout open --json: %v", err)
		}
	})
	if openedName == "" {
		t.Fatal("--json must still open the browser, but execStart was never called")
	}
	if len(openedArgs) == 0 {
		t.Fatalf("browser open args were empty: %v", openedArgs)
	}
	if out == "" {
		t.Fatal("--json should still print the JSON summary to stdout")
	}
}

func TestCheckoutOpen_BrowserOpenErrorPropagates(t *testing.T) {
	oldExec := execStart
	defer func() { execStart = oldExec }()
	execStart = func(name string, argv []string) error {
		return errBrowserOpenFailedForTest
	}

	rt := runtime{}
	ctx := ctxWithRuntime(context.Background(), rt)
	cmd := newCheckoutOpenCmd()
	cmd.SetContext(ctx)

	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("expected browser-open failure to propagate as an error")
	}
}

var errBrowserOpenFailedForTest = errors.New("simulated browser open failure")
