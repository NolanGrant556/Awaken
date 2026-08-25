package mcpclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFixedOriginHeadersStayOnConfiguredOrigin(t *testing.T) {
	var redirectedHeader string
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedHeader = r.Header.Get("X-Plugin")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer redirectTarget.Close()

	var directHeader string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, redirectTarget.URL, http.StatusFound)
			return
		}
		directHeader = r.Header.Get("X-Plugin")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer origin.Close()

	client := buildHTTPClient(false, origin.URL+"/mcp", map[string]string{"X-Plugin": "portable"})
	response, err := client.Get(origin.URL + "/direct")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if directHeader != "portable" {
		t.Fatalf("configured-origin header=%q", directHeader)
	}
	response, err = client.Get(origin.URL + "/redirect")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if redirectedHeader != "" {
		t.Fatalf("configured header leaked across origin: %q", redirectedHeader)
	}
}

func TestFixedOriginHeadersDoNotOverrideHostHeaders(t *testing.T) {
	var received string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer origin.Close()

	client := buildHTTPClient(false, origin.URL, map[string]string{"Authorization": "plugin"})
	request, err := http.NewRequest(http.MethodGet, origin.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "host")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if received != "host" {
		t.Fatalf("host-generated header must win, received %q", received)
	}
}
