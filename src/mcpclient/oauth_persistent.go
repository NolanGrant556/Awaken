package mcpclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

type persistentOAuthHandler struct {
	serverName string
	cfg        AuthFlowConfig
	store      CredentialStore

	mu          sync.Mutex
	tokenSource oauth2.TokenSource
}

func newPersistentOAuthHandler(serverName string, cfg AuthFlowConfig, store CredentialStore) *persistentOAuthHandler {
	return &persistentOAuthHandler{
		serverName: serverName,
		cfg:        cfg,
		store:      store,
	}
}

func (h *persistentOAuthHandler) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.tokenSource != nil {
		return h.tokenSource, nil
	}
	if h.store == nil {
		return nil, nil
	}
	stored, err := h.store.LoadTokenSource(h.serverName)
	if err != nil {
		if err == ErrCredentialNotFound {
			return nil, nil
		}
		return nil, err
	}
	if stored.NeedsReauth {
		return nil, nil
	}
	ts, err := h.wrapStoredTokenSource(ctx, stored)
	if err != nil {
		return nil, err
	}
	h.tokenSource = ts
	return h.tokenSource, nil
}

func (h *persistentOAuthHandler) Authorize(ctx context.Context, req *http.Request, resp *http.Response) error {
	discovered := discoverPersistentAuthState(ctx, req, resp, h.cfg)
	handler, err := buildOAuthHandler(h.cfg)
	if err != nil {
		return err
	}
	if err := handler.Authorize(ctx, req, resp); err != nil {
		return err
	}
	baseTokenSource, err := handler.TokenSource(ctx)
	if err != nil {
		return err
	}
	token, err := baseTokenSource.Token()
	if err != nil {
		return err
	}
	discovered.Token = token
	discovered.NeedsReauth = false
	wrapped, err := h.wrapStoredTokenSource(ctx, discovered)
	if err != nil {
		return err
	}
	if h.store != nil {
		if err := h.store.SaveTokenSource(h.serverName, discovered); err != nil {
			return err
		}
	}
	h.mu.Lock()
	h.tokenSource = wrapped
	h.mu.Unlock()
	return nil
}

func (h *persistentOAuthHandler) wrapStoredTokenSource(ctx context.Context, stored StoredTokenSource) (oauth2.TokenSource, error) {
	if stored.Token == nil {
		return nil, nil
	}
	var base oauth2.TokenSource
	if stored.TokenURL != "" && stored.ClientID != "" {
		cfg := oauth2.Config{
			ClientID:     stored.ClientID,
			ClientSecret: stored.ClientSecret,
			RedirectURL:  stored.RedirectURL,
			Scopes:       append([]string(nil), stored.Scopes...),
			Endpoint: oauth2.Endpoint{
				AuthURL:   stored.AuthURL,
				TokenURL:  stored.TokenURL,
				AuthStyle: oauth2.AuthStyleAutoDetect,
			},
		}
		base = cfg.TokenSource(ctx, stored.Token)
	} else {
		base = oauth2.StaticTokenSource(stored.Token)
	}
	return &persistingTokenSource{
		base: base,
		onToken: func(token *oauth2.Token) error {
			if h.store == nil {
				return nil
			}
			next := stored
			next.Token = token
			next.NeedsReauth = false
			return h.store.SaveTokenSource(h.serverName, next)
		},
	}, nil
}

type persistingTokenSource struct {
	base oauth2.TokenSource

	mu       sync.Mutex
	lastSeen string
	onToken  func(token *oauth2.Token) error
}

func (s *persistingTokenSource) Token() (*oauth2.Token, error) {
	token, err := s.base.Token()
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, nil
	}
	signature := tokenSignature(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	if signature != "" && signature != s.lastSeen {
		if s.onToken != nil {
			if err := s.onToken(token); err != nil {
				return nil, err
			}
		}
		s.lastSeen = signature
	}
	return token, nil
}

func tokenSignature(token *oauth2.Token) string {
	if token == nil {
		return ""
	}
	return strings.Join([]string{
		token.AccessToken,
		token.TokenType,
		token.RefreshToken,
		token.Expiry.UTC().Format(time.RFC3339Nano),
	}, "\x00")
}

func discoverPersistentAuthState(ctx context.Context, req *http.Request, resp *http.Response, cfg AuthFlowConfig) StoredTokenSource {
	stored := StoredTokenSource{
		ServerURL:    firstNonEmpty(strings.TrimSpace(cfg.ServerURL), requestURLString(req)),
		RedirectURL:  strings.TrimSpace(cfg.RedirectURL),
		ClientID:     strings.TrimSpace(cfg.ClientID),
		ClientSecret: strings.TrimSpace(cfg.ClientSecret),
		Scopes:       append([]string(nil), cfg.Scopes...),
	}
	if resp == nil {
		return stored
	}
	challenges, err := oauthex.ParseWWWAuthenticate(resp.Header[http.CanonicalHeaderKey("WWW-Authenticate")])
	if err != nil {
		return stored
	}
	if len(stored.Scopes) == 0 {
		stored.Scopes = scopesFromChallenges(challenges)
	}
	prm, err := discoverProtectedResourceMetadata(ctx, challenges, stored.ServerURL, http.DefaultClient)
	if err != nil || prm == nil || len(prm.AuthorizationServers) == 0 {
		return stored
	}
	asm, err := sdkauth.GetAuthServerMetadata(ctx, prm.AuthorizationServers[0], http.DefaultClient)
	if err != nil {
		return stored
	}
	if asm == nil {
		authServerURL := prm.AuthorizationServers[0]
		stored.AuthURL = authServerURL + "/authorize"
		stored.TokenURL = authServerURL + "/token"
		return stored
	}
	stored.AuthURL = asm.AuthorizationEndpoint
	stored.TokenURL = asm.TokenEndpoint
	return stored
}

func requestURLString(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	return req.URL.String()
}

func scopesFromChallenges(challenges []oauthex.Challenge) []string {
	for _, challenge := range challenges {
		if strings.EqualFold(challenge.Scheme, "bearer") && challenge.Params["scope"] != "" {
			return strings.Fields(challenge.Params["scope"])
		}
	}
	return nil
}

func resourceMetadataURLFromChallenges(challenges []oauthex.Challenge) string {
	for _, challenge := range challenges {
		if value := challenge.Params["resource_metadata"]; value != "" {
			return value
		}
	}
	return ""
}

type prmURL struct {
	URL      string
	Resource string
}

func protectedResourceMetadataURLs(metadataURL, resourceURL string) []prmURL {
	var urls []prmURL
	if metadataURL != "" {
		urls = append(urls, prmURL{URL: metadataURL, Resource: resourceURL})
	}
	resource, err := url.Parse(resourceURL)
	if err != nil {
		return urls
	}
	metadata := *resource
	metadata.Path = "/.well-known/oauth-protected-resource/" + strings.TrimLeft(resource.Path, "/")
	urls = append(urls, prmURL{URL: metadata.String(), Resource: resourceURL})
	metadata.Path = "/.well-known/oauth-protected-resource"
	resource.Path = ""
	urls = append(urls, prmURL{URL: metadata.String(), Resource: resource.String()})
	return urls
}

func discoverProtectedResourceMetadata(ctx context.Context, challenges []oauthex.Challenge, serverURL string, client *http.Client) (*oauthex.ProtectedResourceMetadata, error) {
	for _, candidate := range protectedResourceMetadataURLs(resourceMetadataURLFromChallenges(challenges), serverURL) {
		prm, err := oauthex.GetProtectedResourceMetadata(ctx, candidate.URL, candidate.Resource, client)
		if err != nil || prm == nil {
			continue
		}
		if len(prm.AuthorizationServers) == 0 {
			return nil, fmt.Errorf("protected resource metadata has no authorization servers specified")
		}
		return prm, nil
	}
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return nil, err
	}
	parsed.Path = ""
	return &oauthex.ProtectedResourceMetadata{
		AuthorizationServers: []string{parsed.String()},
		Resource:             serverURL,
	}, nil
}
