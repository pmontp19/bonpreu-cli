package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func TestGetAccountStatus(t *testing.T) {
	page := func(isLoggedIn string) string {
		return `<html><script>window.__INITIAL_STATE__={"session":{"csrf":{"token":"t"},"isLoggedIn":` +
			isLoggedIn + `,"customerSession":{"data":null}}};</script></html>`
	}
	cases := []struct {
		name string
		data string
		want bool
	}{
		{"anonymous", `false`, false},
		{"authenticated", `true`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/" {
					t.Errorf("unexpected path %q", r.URL.Path)
				}
				_, _ = w.Write([]byte(page(tc.data)))
			}))
			defer srv.Close()
			c, _ := client.New(&config.Session{}, nil)
			c.BaseURL = srv.URL

			st, err := GetAccountStatus(context.Background(), c)
			if err != nil {
				t.Fatalf("GetAccountStatus: %v", err)
			}
			if st.Authenticated != tc.want {
				t.Errorf("Authenticated = %v, want %v", st.Authenticated, tc.want)
			}
		})
	}
}
