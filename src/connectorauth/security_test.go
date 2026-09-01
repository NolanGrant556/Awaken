package connectorauth

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type oauthPreflightTransport struct {
	methods []string
}

func (transport *oauthPreflightTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.methods = append(transport.methods, request.Method)
	if request.Method == http.MethodHead {
		if request.Body != nil || request.Header.Get("Authorization") != "" {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, io.EOF
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"access_token":"ok"}`)),
		Request:    request,
	}, nil
}

func TestOAuthPostPreflightsBrokenLoopbackProxyWithoutReplayingCredential(t *testing.T) {
	transport := &oauthPreflightTransport{}
	request, err := http.NewRequest(http.MethodPost, "https://login.microsoftonline.com/common/oauth2/v2.0/token", strings.NewReader("refresh_token=secret"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "sensitive")
	reported := 0
	err = preflightSensitiveOAuthRequest(
		transport,
		request,
		func(*http.Request) (*url.URL, error) { return url.Parse("http://127.0.0.1:7897") },
		func(*http.Request) bool { reported++; return true },
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if strings.Join(transport.methods, ",") != "HEAD,POST" || reported != 1 {
		t.Fatalf("methods=%v reported=%d", transport.methods, reported)
	}
}
