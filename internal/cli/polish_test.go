package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/config"
)

// captureStdout swaps os.Stdout for a pipe, runs fn, and returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	return string(b)
}

func TestSessionSummary_OmitsSecrets(t *testing.T) {
	s := &config.Session{
		Cookies:               map[string]string{"VISITORID": "super-secret-value", "x": "y"},
		CSRFToken:             "csrf-token-should-not-leak",
		RegionID:              "R1",
		DeliveryDestinationID: "D1",
	}
	sum := sessionSummary(s)
	b, err := json.Marshal(sum)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if bytes.Contains(b, []byte("super-secret-value")) {
		t.Errorf("summary leaked a cookie value: %s", b)
	}
	if bytes.Contains(b, []byte("csrf-token-should-not-leak")) {
		t.Errorf("summary leaked the CSRF token: %s", b)
	}
	if sum["cookies"] != 2 {
		t.Errorf("cookies = %v, want 2", sum["cookies"])
	}
	if sum["region"] != "R1" || sum["dest"] != "D1" {
		t.Errorf("region/dest = %v/%v, want R1/D1", sum["region"], sum["dest"])
	}
	if sum["has_csrf"] != true {
		t.Errorf("has_csrf = %v, want true", sum["has_csrf"])
	}
}

func TestCartClear_EmptyJSONEmitsValidJSON(t *testing.T) {
	c, stop := newGuardClient(t, "", "0")
	defer stop()
	rt := runtime{client: c, flags: &Flags{JSON: true}, json: true}
	ctx := context.WithValue(context.Background(), keyRT, &rtHolder{rt: &rt})

	cmd := newCartClearCmd()
	cmd.SetContext(ctx)
	out := captureStdout(t, func() {
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("clear: %v", err)
		}
	})
	var got struct {
		Items int `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("clear --json on empty cart must emit valid JSON, got %q: %v", out, err)
	}
	if got.Items != 0 {
		t.Errorf("items = %d, want 0", got.Items)
	}
}
