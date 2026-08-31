// Package pluginauth owns plugin account setup sessions. It is intentionally
// below the HTTP and Electron layers: browsers only receive an authorization
// URL, while tokens and client secrets remain in Core-owned stores.
package pluginauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"awaken/src/connectors"
	"awaken/src/mcpclient"
	"awaken/src/plugins"
	"awaken/src/plugins/ir"

	"github.com/google/uuid"
	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"golang.org/x/oauth2"
)

const sessionTTL = 10 * time.Minute

var (
	ErrUnsupported = errors.New("plugin setup flow is unsupported")
	ErrStale       = errors.New("plugin setup action is stale")
)

type StartRequest struct {
	PluginDigest           string `json:"pluginDigest"`
	RequirementFingerprint string `json:"requirementFingerprint"`
}

// StartResult is consumed only by Electron Main. UserCode is deliberately
// omitted from all later session reads so it cannot leak into Renderer state.
type StartResult struct {
	SessionID        string     `json:"sessionId,omitempty"`
	Status           string     `json:"status"`
	AuthorizationURL string     `json:"authorizationUrl,omitempty"`
	VerificationURL  string     `json:"verificationUrl,omitempty"`
	UserCode         string     `json:"userCode,omitempty"`
	DisplayUserCode  bool       `json:"displayUserCode,omitempty"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
}

type SessionView struct {
	SessionID    string     `json:"sessionId"`
	PluginName   string     `json:"pluginName"`
	SetupID      string     `json:"setupId"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
}

type credentialReference struct {
	PluginName  string `json:"p"`
	SetupID     string `json:"s"`
	Fingerprint string `json:"f"`
	Scheme      string `json:"h,omitempty"`
}

type session struct {
	mu               sync.RWMutex
	id               string
	pluginName       string
	setupID          string
	fingerprint      string
	identity         string
	status           string
	errorText        string
	expiresAt        time.Time
	cancel           context.CancelFunc
	listener         net.Listener
	server           *http.Server
	mcpSession       string
	mcpServerName    string
	connectorSession string
	connectorID      string
}

type hostConnectorAuth interface {
	ConnectorStatus(connectorID string) (connectors.ConnectorStatus, error)
	Start(ctx context.Context, connectorID string) (connectors.ConnectorAuthStartResult, error)
	Session(sessionID string) (connectors.ConnectorAuthSession, error)
	Cancel(sessionID string) (connectors.ConnectorAuthSession, error)
	Disconnect(ctx context.Context, connectorID string) error
}

func (s *session) view() SessionView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	expires := s.expiresAt
	return SessionView{SessionID: s.id, PluginName: s.pluginName, SetupID: s.setupID, Status: s.status, ErrorMessage: s.errorText, ExpiresAt: &expires}
}

func (s *session) finish(status string, err error) {
	s.mu.Lock()
	s.status = status
	if err != nil {
		s.errorText = err.Error()
	} else {
		s.errorText = ""
	}
	s.mu.Unlock()
}

// Service is both the setup mutation owner and the read-only readiness
// provider consumed by plugins.Store.
type Service struct {
	plugins               *plugins.Store
	mcp                   *mcpclient.Manager
	connectors            *connectors.Registry
	hostConnectors        hostConnectorAuth
	credentials           mcpclient.CredentialStore
	httpClient            *http.Client
	accounts              *accountIndexStore
	deviceMinimumInterval time.Duration

	mu       sync.RWMutex
	sessions map[string]*session
}

// SetHostConnectorAuth wires the Core-owned shared Connector account broker.
// Plugin setup can observe and initiate the account flow, but never receives
// the connector's token or ownership of its lifecycle.
func (s *Service) SetHostConnectorAuth(auth hostConnectorAuth) {
	s.hostConnectors = auth
	if s.plugins != nil {
		s.plugins.RefreshSetupReadiness()
	}
}

func NewService(pluginStore *plugins.Store, mcp *mcpclient.Manager, registry *connectors.Registry, credentials mcpclient.CredentialStore) *Service {
	if credentials == nil {
		credentials = mcpclient.NewPersistentCredentialStore()
	}
	service := &Service{
		plugins: pluginStore, mcp: mcp, connectors: registry, credentials: credentials,
		httpClient: newSafeHTTPClient(), sessions: make(map[string]*session),
		accounts: newAccountIndexStore(), deviceMinimumInterval: 5 * time.Second,
	}
	if pluginStore != nil {
		pluginStore.SetSetupReadinessProvider(service)
		pluginStore.SubscribeChanges(func() {
			go func() {
				service.reconcileCredentialIndex()
				service.reconcileMCPBindings(true)
			}()
		})
		go func() {
			service.reconcileCredentialIndex()
			time.Sleep(500 * time.Millisecond)
			service.reconcileMCPBindings(true)
		}()
	}
	return service
}

func (s *Service) recordExistingCredential(pluginName string, requirement ir.SetupRequirement, identity string) error {
	previous, hadPrevious, err := s.accounts.upsert(pluginName, requirement.ID, requirement.Fingerprint, identity)
	if err != nil {
		return err
	}
	if hadPrevious && previous.Identity != identity {
		if clearErr := s.credentials.ClearTokenSource(previous.Identity); clearErr != nil {
			_ = s.accounts.removeOrTombstone(previous, true)
		}
	}
	return nil
}

func (s *Service) saveCredential(pluginName string, requirement ir.SetupRequirement, stored mcpclient.StoredTokenSource) error {
	identity := pluginCredentialIdentity(pluginName, requirement)
	stored.CredentialRef = ""
	if err := s.credentials.SaveTokenSource(identity, stored); err != nil {
		return err
	}
	previous, hadPrevious, err := s.accounts.upsert(pluginName, requirement.ID, requirement.Fingerprint, identity)
	if err != nil {
		_ = s.credentials.ClearTokenSource(identity)
		return err
	}
	if hadPrevious && previous.Identity != identity {
		if clearErr := s.credentials.ClearTokenSource(previous.Identity); clearErr != nil {
			_ = s.accounts.removeOrTombstone(previous, true)
		}
	}
	return nil
}

func (s *Service) reconcileCredentialIndex() {
	entries, err := s.accounts.entries()
	if err != nil || len(entries) == 0 {
		return
	}
	current := map[string]string{}
	if s.plugins != nil {
		if installed, listErr := s.plugins.List(); listErr == nil {
			for _, plugin := range installed {
				for _, requirement := range plugin.Setup {
					current[accountIndexKey(plugin.Name, requirement.ID)] = requirement.Fingerprint
				}
			}
		}
	}
	for _, entry := range entries {
		fingerprint, exists := current[accountIndexKey(entry.PluginName, entry.SetupID)]
		if !entry.Tombstone && exists && fingerprint == entry.Fingerprint {
			continue
		}
		if clearErr := s.credentials.ClearTokenSource(entry.Identity); clearErr != nil {
			_ = s.accounts.removeOrTombstone(entry, true)
			continue
		}
		_ = s.accounts.removeOrTombstone(entry, false)
	}
}

type boundOAuthHandler struct {
	service     *Service
	pluginName  string
	requirement ir.SetupRequirement
}

type tokenSourceFunc func() (*oauth2.Token, error)

func (f tokenSourceFunc) Token() (*oauth2.Token, error) { return f() }

var _ sdkauth.OAuthHandler = (*boundOAuthHandler)(nil)

func (h *boundOAuthHandler) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	token, err := h.service.ResolveAccessToken(ctx, h.pluginName, h.requirement)
	if err != nil {
		if errors.Is(err, mcpclient.ErrCredentialNotFound) {
			return nil, nil
		}
		return nil, err
	}
	_ = token
	return tokenSourceFunc(func() (*oauth2.Token, error) {
		resolveCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		value, resolveErr := h.service.ResolveAccessToken(resolveCtx, h.pluginName, h.requirement)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return &oauth2.Token{AccessToken: value, TokenType: "Bearer"}, nil
	}), nil
}

func (h *boundOAuthHandler) Authorize(_ context.Context, _ *http.Request, response *http.Response) error {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	identity := pluginCredentialIdentity(h.pluginName, h.requirement)
	stored, err := h.service.credentials.LoadTokenSource(identity)
	if err == nil {
		stored.NeedsReauth = true
		_ = h.service.credentials.SaveTokenSource(identity, stored)
	}
	return &mcpclient.NeedsAuthError{ServerName: h.service.mcpServerName(h.pluginName, h.requirement), Reason: "plugin OAuth credential was rejected"}
}

func (s *Service) reconcileMCPBindings(restart bool) {
	if s.plugins == nil || s.mcp == nil {
		return
	}
	installed, err := s.plugins.List()
	if err != nil {
		return
	}
	for _, plugin := range installed {
		for _, requirement := range plugin.Setup {
			if requirement.Kind != ir.SetupOAuthPKCE && requirement.Kind != ir.SetupOAuthDevice {
				continue
			}
			bound := false
			for _, binding := range requirement.Bindings {
				if binding.Target == ir.SetupBindingMCPTransport {
					bound = true
					break
				}
			}
			if !bound {
				continue
			}
			serverName := s.mcpServerName(plugin.Name, requirement)
			if serverName == "" {
				continue
			}
			handler := &boundOAuthHandler{service: s, pluginName: plugin.Name, requirement: requirement}
			if err := s.mcp.InstallServerAuthHandler(serverName, handler); err != nil {
				continue
			}
			if restart {
				_ = s.mcp.ForceReconnect(context.Background(), serverName)
			}
		}
	}
}

func (s *Service) activateMCPBinding(pluginName string, requirement ir.SetupRequirement) {
	if s.mcp == nil || strings.TrimSpace(requirement.MCPServerName) == "" {
		return
	}
	serverName := s.mcpServerName(pluginName, requirement)
	if serverName == "" {
		return
	}
	boundToMCP := false
	needsAuthHandler := false
	for _, binding := range requirement.Bindings {
		switch binding.Target {
		case ir.SetupBindingMCPTransport:
			boundToMCP = true
			needsAuthHandler = true
		case ir.SetupBindingHTTPHeader, ir.SetupBindingProcessEnv:
			boundToMCP = true
		}
	}
	if !boundToMCP {
		return
	}
	restarted := false
	if s.plugins != nil {
		for _, scoped := range s.plugins.PluginMCPServers() {
			if scoped.Name != serverName {
				continue
			}
			if s.mcp.RestartWithConfig(context.Background(), scoped) == nil {
				restarted = true
			}
			break
		}
	}
	if needsAuthHandler {
		_ = s.mcp.InstallServerAuthHandler(serverName, &boundOAuthHandler{service: s, pluginName: pluginName, requirement: requirement})
		_ = s.mcp.ForceReconnect(context.Background(), serverName)
	} else if !restarted {
		_ = s.mcp.ForceReconnect(context.Background(), serverName)
	}
}

func pluginCredentialIdentity(pluginName string, requirement ir.SetupRequirement) string {
	sum := sha256.Sum256([]byte(pluginName + "\x00" + requirement.ID + "\x00" + requirement.Fingerprint))
	return "plugin:" + hex.EncodeToString(sum[:])
}

func (s *Service) SetupCredentialReference(pluginName string, requirement ir.SetupRequirement, binding ir.SetupBinding) (string, error) {
	if requirement.Kind != ir.SetupSecret && requirement.Kind != ir.SetupOAuthPKCE && requirement.Kind != ir.SetupOAuthDevice {
		return "", ErrUnsupported
	}
	payload, err := json.Marshal(credentialReference{PluginName: pluginName, SetupID: requirement.ID, Fingerprint: requirement.Fingerprint, Scheme: strings.TrimSpace(binding.Scheme)})
	if err != nil {
		return "", err
	}
	return "plugin-auth:" + base64.RawURLEncoding.EncodeToString(payload), nil
}

// ResolveHookSecret implements hookruntime.SecretResolver without importing
// hookruntime. Only opaque references minted by SetupCredentialReference are
// accepted; arbitrary keychain account names cannot be requested by a plugin.
func (s *Service) ResolveHookSecret(ctx context.Context, ref string) (string, error) {
	if !strings.HasPrefix(ref, "plugin-auth:") || s.plugins == nil {
		return "", mcpclient.ErrCredentialNotFound
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ref, "plugin-auth:"))
	if err != nil {
		return "", mcpclient.ErrCredentialNotFound
	}
	var reference credentialReference
	if err := json.Unmarshal(raw, &reference); err != nil {
		return "", mcpclient.ErrCredentialNotFound
	}
	plugin, err := s.plugins.Get(reference.PluginName)
	if err != nil || !plugin.IntegrityValid {
		return "", mcpclient.ErrCredentialNotFound
	}
	var requirement *ir.SetupRequirement
	for index := range plugin.Setup {
		if plugin.Setup[index].ID == reference.SetupID && plugin.Setup[index].Fingerprint == reference.Fingerprint {
			requirement = &plugin.Setup[index]
			break
		}
	}
	if requirement == nil {
		return "", ErrStale
	}
	var value string
	switch requirement.Kind {
	case ir.SetupSecret:
		if requirement.Field == nil {
			return "", mcpclient.ErrCredentialNotFound
		}
		value, err = s.plugins.ResolveVariableValue(reference.PluginName, requirement.Field.Key)
	case ir.SetupOAuthPKCE, ir.SetupOAuthDevice:
		value, err = s.ResolveAccessToken(ctx, reference.PluginName, *requirement)
	default:
		err = ErrUnsupported
	}
	if err != nil {
		return "", err
	}
	if reference.Scheme != "" {
		value = reference.Scheme + " " + value
	}
	return value, nil
}

func (s *Service) ResolveMCPSecret(ctx context.Context, ref string) (string, error) {
	return s.ResolveHookSecret(ctx, ref)
}

func (s *Service) SetupStatus(pluginName string, requirement ir.SetupRequirement) (plugins.SetupStatus, string) {
	switch requirement.Kind {
	case ir.SetupAppConnector:
		return plugins.SetupUnsupported, "Awaken 当前未提供该 Connector ID 的目录、授权和运行时。"
	case ir.SetupHostConnector:
		if s.hostConnectors == nil || strings.TrimSpace(requirement.ConnectorID) == "" {
			return plugins.SetupUnsupported, "该 Awaken 宿主连接器未注册。"
		}
		connectorStatus, err := s.hostConnectors.ConnectorStatus(requirement.ConnectorID)
		if err != nil {
			return plugins.SetupUnsupported, "该 Awaken 宿主连接器不可用。"
		}
		switch connectorStatus.State {
		case connectors.StateConnected:
			return plugins.SetupReady, "宿主连接器账号已连接；多个插件可以安全复用该账号能力。"
		case connectors.StatePending:
			return plugins.SetupAuthorizing, "正在连接宿主连接器账号。"
		case connectors.StateFailed:
			return plugins.SetupFailed, firstSafeMessage(connectorStatus.LastError, "宿主连接器账号连接失败。")
		case connectors.StateDisconnected:
			return plugins.SetupFailed, "宿主连接器账号已就绪，但工具运行时尚未连接。"
		default:
			return plugins.SetupNeedsAuth, "需要连接宿主连接器账号。"
		}
	case ir.SetupCLILogin:
		if s.connectors == nil {
			return plugins.SetupUnsupported, "宿主 CLI Connector 未注册。"
		}
		status, err := s.connectors.Status(requirement.ProviderID)
		if err != nil || status.Kind != connectors.KindCLI {
			return plugins.SetupUnsupported, "未知的宿主 CLI Connector。"
		}
		if status.CLIReadiness != nil {
			switch status.CLIReadiness.Authentication {
			case connectors.CLIAuthenticationAuthorized:
				return plugins.SetupReady, "宿主 CLI 账号已登录。"
			case connectors.CLIAuthenticationInProgress:
				return plugins.SetupAuthorizing, "正在由宿主 CLI 完成登录。"
			case connectors.CLIAuthenticationFailed:
				return plugins.SetupFailed, "宿主 CLI 登录失败。"
			}
		}
		return plugins.SetupNeedsAuth, "请在连接器设置中登录宿主 CLI 账号。"
	case ir.SetupExternal:
		if s.boundComponentsReady(pluginName, requirement) {
			return plugins.SetupReady, "关联组件已通过就绪检测。"
		}
		return plugins.SetupNeedsAuth, "请完成外部网页设置后重新检测。"
	case ir.SetupMCPOAuth:
		return s.mcpStatus(pluginName, requirement)
	case ir.SetupOAuthPKCE, ir.SetupOAuthDevice:
		if active := s.activeSession(pluginName, requirement.ID, requirement.Fingerprint); active != nil {
			view := active.view()
			return statusFromString(view.Status), view.ErrorMessage
		}
		stored, err := s.credentials.LoadTokenSource(pluginCredentialIdentity(pluginName, requirement))
		if err != nil {
			if errors.Is(err, mcpclient.ErrCredentialNotFound) {
				return plugins.SetupNeedsAuth, "需要连接账号。"
			}
			return plugins.SetupFailed, "无法读取本机凭据状态。"
		}
		if stored.NeedsReauth || stored.Token == nil {
			return plugins.SetupNeedsAuth, "账号需要重新授权。"
		}
		if !stored.Token.Expiry.IsZero() && time.Now().After(stored.Token.Expiry) && stored.Token.RefreshToken == "" {
			return plugins.SetupExpired, "授权已过期。"
		}
		return plugins.SetupReady, "账号已连接。"
	default:
		return plugins.SetupReady, ""
	}
}

func statusFromString(value string) plugins.SetupStatus {
	switch value {
	case "authorizing":
		return plugins.SetupAuthorizing
	case "verifying":
		return plugins.SetupVerifying
	case "ready":
		return plugins.SetupReady
	case "expired":
		return plugins.SetupExpired
	case "failed", "cancelled":
		return plugins.SetupFailed
	default:
		return plugins.SetupNeedsAuth
	}
}

func (s *Service) mcpStatus(pluginName string, requirement ir.SetupRequirement) (plugins.SetupStatus, string) {
	serverName := s.mcpServerName(pluginName, requirement)
	if serverName == "" || s.mcp == nil {
		return plugins.SetupUnsupported, "关联的远程 MCP 服务不可用。"
	}
	if active, ok := s.mcp.ActiveAuthSessionForServer(serverName); ok {
		return statusFromString(active.Status), active.ErrorMessage
	}
	if snapshot, err := s.mcp.SnapshotServer(serverName); err == nil {
		switch snapshot.State {
		case mcpclient.StateConnected:
			return plugins.SetupReady, "远程 MCP 已连接。"
		case mcpclient.StateNeedsAuth:
			return plugins.SetupNeedsAuth, "远程 MCP 需要账号授权。"
		}
	}
	if present, err := s.mcp.HasServerCredentials(serverName); err == nil && present {
		return plugins.SetupReady, "远程 MCP 凭据已保存，等待重连。"
	}
	if requirement.Required {
		return plugins.SetupNeedsAuth, "远程 MCP 需要账号授权。"
	}
	return plugins.SetupReady, "远程 MCP 未声明强制授权。"
}

func (s *Service) mcpServerName(pluginName string, requirement ir.SetupRequirement) string {
	if strings.TrimSpace(requirement.MCPServerName) != "" {
		return plugins.ServerIdentity(pluginName, requirement.MCPServerName)
	}
	return ""
}

func (s *Service) boundComponentsReady(pluginName string, requirement ir.SetupRequirement) bool {
	serverName := s.mcpServerName(pluginName, requirement)
	if serverName != "" && s.mcp != nil {
		snapshot, err := s.mcp.SnapshotServer(serverName)
		return err == nil && snapshot.State == mcpclient.StateConnected
	}
	return false
}

func (s *Service) Start(ctx context.Context, pluginName, setupID string, request StartRequest) (StartResult, error) {
	if s.plugins == nil {
		return StartResult{}, errors.New("plugin store is unavailable")
	}
	_, requirement, err := s.plugins.ResolveSetupRequirement(pluginName, setupID, request.PluginDigest, request.RequirementFingerprint)
	if err != nil {
		return StartResult{}, errors.Join(ErrStale, err)
	}
	switch requirement.Kind {
	case ir.SetupMCPOAuth:
		return s.startMCP(ctx, pluginName, requirement)
	case ir.SetupHostConnector:
		return s.startHostConnector(ctx, pluginName, requirement)
	case ir.SetupOAuthPKCE:
		return s.startPKCE(pluginName, requirement)
	case ir.SetupOAuthDevice:
		return s.startDevice(pluginName, requirement)
	case ir.SetupExternal:
		if err := ValidateActionURL(requirement.ExternalURL); err != nil {
			return StartResult{}, err
		}
		return StartResult{Status: string(plugins.SetupNeedsAuth), AuthorizationURL: requirement.ExternalURL}, nil
	case ir.SetupCLILogin:
		if status, _ := s.SetupStatus(pluginName, requirement); status == plugins.SetupUnsupported {
			return StartResult{}, ErrUnsupported
		}
		return StartResult{Status: string(plugins.SetupNeedsAuth)}, nil
	default:
		return StartResult{}, ErrUnsupported
	}
}

func (s *Service) startHostConnector(ctx context.Context, pluginName string, requirement ir.SetupRequirement) (StartResult, error) {
	if s.hostConnectors == nil || strings.TrimSpace(requirement.ConnectorID) == "" {
		return StartResult{}, ErrUnsupported
	}
	result, err := s.hostConnectors.Start(ctx, requirement.ConnectorID)
	if err != nil {
		return StartResult{}, err
	}
	if strings.TrimSpace(result.SessionID) == "" {
		if s.plugins != nil {
			s.plugins.RefreshSetupReadiness()
		}
		return StartResult{Status: result.Status, AuthorizationURL: result.AuthorizationURL, ExpiresAt: result.ExpiresAt}, nil
	}
	entry := &session{
		id: uuid.NewString(), pluginName: pluginName, setupID: requirement.ID,
		fingerprint: requirement.Fingerprint, status: result.Status,
		expiresAt: derefTime(result.ExpiresAt), connectorSession: result.SessionID,
		connectorID: requirement.ConnectorID,
	}
	s.putSession(entry)
	go s.watchHostConnectorSession(entry)
	return StartResult{
		SessionID: entry.id, Status: result.Status, AuthorizationURL: result.AuthorizationURL,
		ExpiresAt: result.ExpiresAt,
	}, nil
}

func (s *Service) watchHostConnectorSession(entry *session) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer func() {
		ticker.Stop()
		s.scheduleSessionRemoval(entry)
	}()
	for {
		view, err := s.hostConnectors.Session(entry.connectorSession)
		if err != nil {
			entry.finish("failed", errors.New("宿主连接器授权会话不可用"))
			return
		}
		entry.finish(view.Status, errorFromText(view.ErrorMessage))
		if view.Status == "authorizing" || view.Status == "verifying" {
			<-ticker.C
			continue
		}
		if s.plugins != nil {
			s.plugins.RefreshSetupReadiness()
		}
		return
	}
}

func (s *Service) startMCP(ctx context.Context, pluginName string, requirement ir.SetupRequirement) (StartResult, error) {
	serverName := s.mcpServerName(pluginName, requirement)
	if serverName == "" || s.mcp == nil {
		return StartResult{}, ErrUnsupported
	}
	request := mcpclient.AuthRequest{}
	if requirement.OAuth != nil {
		request.ClientID = requirement.OAuth.ClientID
		request.ClientMetadataURL = requirement.OAuth.ClientMetadataURL
		request.Scopes = append([]string(nil), requirement.OAuth.Scopes...)
	}
	result, err := s.mcp.StartAuth(ctx, serverName, request)
	if err != nil {
		return StartResult{}, err
	}
	entry := &session{id: result.SessionID, pluginName: pluginName, setupID: requirement.ID, fingerprint: requirement.Fingerprint, status: "authorizing", expiresAt: derefTime(result.ExpiresAt), mcpSession: result.SessionID, mcpServerName: serverName}
	s.putSession(entry)
	go s.watchMCPAuthSession(entry)
	return StartResult{SessionID: result.SessionID, Status: "authorizing", AuthorizationURL: result.AuthorizationURL, ExpiresAt: result.ExpiresAt}, nil
}

func (s *Service) watchMCPAuthSession(entry *session) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer func() {
		ticker.Stop()
		s.scheduleSessionRemoval(entry)
	}()
	for {
		view, err := s.mcp.AuthSession(entry.mcpSession)
		if err != nil {
			return
		}
		entry.finish(view.Status, errorFromText(view.ErrorMessage))
		if view.Status == "authorizing" || view.Status == "verifying" {
			<-ticker.C
			continue
		}
		if view.Status == "ready" && s.plugins != nil {
			if plugin, err := s.plugins.Get(entry.pluginName); err == nil {
				for _, requirement := range plugin.Setup {
					if requirement.ID != entry.setupID || requirement.Fingerprint != entry.fingerprint {
						continue
					}
					if identity, identityErr := s.mcp.ServerCredentialIdentity(entry.mcpServerName); identityErr == nil {
						_ = s.recordExistingCredential(entry.pluginName, requirement, identity)
					}
					break
				}
			}
			s.plugins.RefreshSetupReadiness()
		}
		return
	}
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Now().UTC().Add(sessionTTL)
	}
	return *value
}

func (s *Service) putSession(value *session) {
	s.mu.Lock()
	for _, existing := range s.sessions {
		if existing.pluginName == value.pluginName && existing.setupID == value.setupID {
			if existing.cancel != nil {
				existing.cancel()
			}
		}
	}
	s.sessions[value.id] = value
	s.mu.Unlock()
}

func (s *Service) scheduleSessionRemoval(entry *session) {
	if entry == nil {
		return
	}
	time.AfterFunc(15*time.Minute, func() {
		s.mu.Lock()
		if s.sessions[entry.id] == entry {
			delete(s.sessions, entry.id)
		}
		s.mu.Unlock()
	})
}

func (s *Service) activeSession(pluginName, setupID, fingerprint string) *session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, candidate := range s.sessions {
		view := candidate.view()
		if view.PluginName == pluginName && view.SetupID == setupID && candidate.fingerprint == fingerprint && (view.Status == "authorizing" || view.Status == "verifying") {
			return candidate
		}
	}
	return nil
}

func (s *Service) Session(sessionID string) (SessionView, error) {
	s.mu.RLock()
	entry := s.sessions[strings.TrimSpace(sessionID)]
	s.mu.RUnlock()
	if entry == nil {
		return SessionView{}, errors.New("plugin auth session not found")
	}
	if entry.mcpSession != "" && s.mcp != nil {
		view, err := s.mcp.AuthSession(entry.mcpSession)
		if err == nil {
			entry.finish(view.Status, errorFromText(view.ErrorMessage))
		}
	}
	if entry.connectorSession != "" && s.hostConnectors != nil {
		view, err := s.hostConnectors.Session(entry.connectorSession)
		if err == nil {
			entry.finish(view.Status, errorFromText(view.ErrorMessage))
		}
	}
	return entry.view(), nil
}

func errorFromText(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return errors.New(value)
}

func (s *Service) Cancel(sessionID string) (SessionView, error) {
	s.mu.RLock()
	entry := s.sessions[strings.TrimSpace(sessionID)]
	s.mu.RUnlock()
	if entry == nil {
		return SessionView{}, errors.New("plugin auth session not found")
	}
	if entry.mcpSession != "" && s.mcp != nil {
		_, _ = s.mcp.CancelAuthSession(entry.mcpSession)
	}
	if entry.connectorSession != "" && s.hostConnectors != nil {
		_, _ = s.hostConnectors.Cancel(entry.connectorSession)
	}
	if entry.cancel != nil {
		entry.cancel()
	}
	if entry.server != nil {
		_ = entry.server.Close()
	}
	entry.finish("cancelled", errors.New("authorization cancelled"))
	return entry.view(), nil
}

func (s *Service) Disconnect(pluginName, setupID, digest, fingerprint string) error {
	_, requirement, err := s.plugins.ResolveSetupRequirement(pluginName, setupID, digest, fingerprint)
	if err != nil {
		return errors.Join(ErrStale, err)
	}
	if requirement.Kind == ir.SetupMCPOAuth {
		serverName := s.mcpServerName(pluginName, requirement)
		if serverName == "" || s.mcp == nil {
			return ErrUnsupported
		}
		if err := s.mcp.ClearServerCredentials(serverName); err != nil {
			return err
		}
		if identity, identityErr := s.mcp.ServerCredentialIdentity(serverName); identityErr == nil {
			if entries, indexErr := s.accounts.entries(); indexErr == nil {
				for _, entry := range entries {
					if entry.Identity == identity {
						_ = s.accounts.removeOrTombstone(entry, false)
					}
				}
			}
		}
		s.plugins.RefreshSetupReadiness()
		return nil
	}
	if requirement.Kind == ir.SetupHostConnector {
		if s.hostConnectors == nil || strings.TrimSpace(requirement.ConnectorID) == "" {
			return ErrUnsupported
		}
		if err := s.hostConnectors.Disconnect(context.Background(), requirement.ConnectorID); err != nil {
			return err
		}
		s.plugins.RefreshSetupReadiness()
		return nil
	}
	identity := pluginCredentialIdentity(pluginName, requirement)
	if err := s.credentials.ClearTokenSource(identity); err != nil {
		return err
	}
	if entries, err := s.accounts.entries(); err == nil {
		for _, entry := range entries {
			if entry.PluginName == pluginName && entry.SetupID == setupID && entry.Identity == identity {
				_ = s.accounts.removeOrTombstone(entry, false)
			}
		}
	}
	s.plugins.RefreshSetupReadiness()
	return nil
}

// CleanupPlugin cancels live authorization work and removes every credential
// reachable from the currently installed declaration. It is called before the
// package record is removed, while immutable setup fingerprints still exist.
func (s *Service) CleanupPlugin(pluginName string) error {
	if s.plugins == nil {
		return nil
	}
	plugin, err := s.plugins.Get(pluginName)
	if err != nil {
		return err
	}
	s.mu.RLock()
	var activeIDs []string
	for id, candidate := range s.sessions {
		if candidate.pluginName == pluginName {
			activeIDs = append(activeIDs, id)
		}
	}
	s.mu.RUnlock()
	var cleanupErrors []error
	for _, id := range activeIDs {
		if _, cancelErr := s.Cancel(id); cancelErr != nil {
			cleanupErrors = append(cleanupErrors, cancelErr)
		}
	}
	indexed := map[string]bool{}
	if entries, indexErr := s.accounts.entries(); indexErr == nil {
		for _, entry := range entries {
			if entry.PluginName != pluginName {
				continue
			}
			indexed[entry.Identity] = true
			if clearErr := s.credentials.ClearTokenSource(entry.Identity); clearErr != nil {
				if tombstoneErr := s.accounts.removeOrTombstone(entry, true); tombstoneErr != nil {
					cleanupErrors = append(cleanupErrors, errors.Join(clearErr, tombstoneErr))
				}
			} else if removeErr := s.accounts.removeOrTombstone(entry, false); removeErr != nil {
				cleanupErrors = append(cleanupErrors, removeErr)
			}
		}
	} else {
		cleanupErrors = append(cleanupErrors, indexErr)
	}
	for _, requirement := range plugin.Setup {
		if requirement.Kind == ir.SetupHostConnector || requirement.Kind == ir.SetupAppConnector || requirement.Kind == ir.SetupCLILogin || requirement.Kind == ir.SetupExternal {
			continue
		}
		if requirement.Kind == ir.SetupMCPOAuth {
			if serverName := s.mcpServerName(pluginName, requirement); serverName != "" && s.mcp != nil {
				identity, _ := s.mcp.ServerCredentialIdentity(serverName)
				if indexed[identity] {
					continue
				}
				if clearErr := s.mcp.ClearServerCredentials(serverName); clearErr != nil {
					cleanupErrors = append(cleanupErrors, clearErr)
				}
			}
			continue
		}
		if requirement.Kind == ir.SetupOAuthPKCE || requirement.Kind == ir.SetupOAuthDevice {
			identity := pluginCredentialIdentity(pluginName, requirement)
			if indexed[identity] {
				continue
			}
			if clearErr := s.credentials.ClearTokenSource(identity); clearErr != nil {
				cleanupErrors = append(cleanupErrors, clearErr)
			}
		}
	}
	return errors.Join(cleanupErrors...)
}

func firstSafeMessage(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

// ResolveAccessToken refreshes and returns a token only to Core-owned binding
// injection code. It is never exposed by an HTTP handler.
func (s *Service) ResolveAccessToken(ctx context.Context, pluginName string, requirement ir.SetupRequirement) (string, error) {
	identity := pluginCredentialIdentity(pluginName, requirement)
	stored, err := s.credentials.LoadTokenSource(identity)
	if err != nil {
		return "", err
	}
	if stored.Token == nil {
		return "", mcpclient.ErrCredentialNotFound
	}
	config := oauth2.Config{ClientID: stored.ClientID, ClientSecret: stored.ClientSecret, Scopes: stored.Scopes, Endpoint: oauth2.Endpoint{AuthURL: stored.AuthURL, TokenURL: stored.TokenURL}}
	clientCtx := context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	refreshed, err := config.TokenSource(clientCtx, stored.Token).Token()
	if err != nil {
		stored.NeedsReauth = true
		_ = s.credentials.SaveTokenSource(identity, stored)
		return "", err
	}
	if refreshed.AccessToken != stored.Token.AccessToken || !refreshed.Expiry.Equal(stored.Token.Expiry) {
		stored.Token = refreshed
		if err := s.credentials.SaveTokenSource(identity, stored); err != nil {
			return "", err
		}
	}
	return refreshed.AccessToken, nil
}

func randomURLSafe(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (s *Service) oauthClient(pluginName string, requirement ir.SetupRequirement) (clientID, clientSecret string, err error) {
	spec := requirement.OAuth
	if spec == nil {
		return "", "", errors.New("OAuth declaration is missing")
	}
	clientID = strings.TrimSpace(spec.ClientID)
	if clientID == "" {
		clientID, err = s.plugins.ResolveVariableValue(pluginName, spec.ClientIDVariable)
		if err != nil {
			return "", "", err
		}
	}
	if spec.ClientSecretVariable != "" {
		clientSecret, err = s.plugins.ResolveVariableValue(pluginName, spec.ClientSecretVariable)
		if err != nil {
			return "", "", err
		}
	}
	return clientID, clientSecret, nil
}

type discoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

func (s *Service) resolvePKCEEndpoints(ctx context.Context, spec ir.OAuthSetupSpec) (string, string, error) {
	if spec.AuthorizationEndpoint != "" && spec.TokenEndpoint != "" {
		if err := ValidateActionURL(spec.AuthorizationEndpoint); err != nil {
			return "", "", err
		}
		if err := ValidateActionURL(spec.TokenEndpoint); err != nil {
			return "", "", err
		}
		return spec.AuthorizationEndpoint, spec.TokenEndpoint, nil
	}
	issuer := strings.TrimRight(spec.Issuer, "/")
	if err := ValidateActionURL(issuer); err != nil {
		return "", "", err
	}
	issuerURL, _ := url.Parse(issuer)
	discovery := *issuerURL
	discovery.Path = "/.well-known/oauth-authorization-server" + strings.TrimSuffix(issuerURL.EscapedPath(), "/")
	discovery.RawPath = ""
	discovery.RawQuery = ""
	discoveryURL := discovery.String()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("discover OAuth issuer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("discover OAuth issuer: HTTP %d", resp.StatusCode)
	}
	var document discoveryDocument
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&document); err != nil {
		return "", "", fmt.Errorf("decode OAuth discovery: %w", err)
	}
	if strings.TrimRight(document.Issuer, "/") != issuer {
		return "", "", errors.New("OAuth discovery issuer did not match the declaration")
	}
	if err := ValidateActionURL(document.AuthorizationEndpoint); err != nil {
		return "", "", err
	}
	if err := ValidateActionURL(document.TokenEndpoint); err != nil {
		return "", "", err
	}
	return document.AuthorizationEndpoint, document.TokenEndpoint, nil
}

func (s *Service) startPKCE(pluginName string, requirement ir.SetupRequirement) (StartResult, error) {
	clientID, clientSecret, err := s.oauthClient(pluginName, requirement)
	if err != nil {
		return StartResult{}, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return StartResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), sessionTTL)
	authURL, tokenURL, err := s.resolvePKCEEndpoints(ctx, *requirement.OAuth)
	if err != nil {
		cancel()
		_ = listener.Close()
		return StartResult{}, err
	}
	state, err := randomURLSafe(32)
	if err != nil {
		cancel()
		_ = listener.Close()
		return StartResult{}, err
	}
	verifier, err := randomURLSafe(64)
	if err != nil {
		cancel()
		_ = listener.Close()
		return StartResult{}, err
	}
	challengeRaw := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeRaw[:])
	callbackURL := "http://" + listener.Addr().String() + "/callback"
	config := oauth2.Config{ClientID: clientID, ClientSecret: clientSecret, RedirectURL: callbackURL, Scopes: append([]string(nil), requirement.OAuth.Scopes...), Endpoint: oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL}}
	options := []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("code_challenge", challenge), oauth2.SetAuthURLParam("code_challenge_method", "S256")}
	if requirement.OAuth.Resource != "" {
		options = append(options, oauth2.SetAuthURLParam("resource", requirement.OAuth.Resource))
	}
	if requirement.OAuth.Audience != "" {
		options = append(options, oauth2.SetAuthURLParam("audience", requirement.OAuth.Audience))
	}
	authorizationURL := config.AuthCodeURL(state, options...)
	if err := ValidateActionURL(authorizationURL); err != nil {
		cancel()
		_ = listener.Close()
		return StartResult{}, err
	}
	entry := &session{id: uuid.NewString(), pluginName: pluginName, setupID: requirement.ID, fingerprint: requirement.Fingerprint, identity: pluginCredentialIdentity(pluginName, requirement), status: "authorizing", expiresAt: time.Now().UTC().Add(sessionTTL), cancel: cancel, listener: listener}
	codeCh := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "OAuth state did not match", http.StatusBadRequest)
			return
		}
		if providerError := strings.TrimSpace(r.URL.Query().Get("error")); providerError != "" {
			entry.finish("failed", fmt.Errorf("OAuth provider returned %s", providerError))
			http.Error(w, "Authorization failed. You can close this window.", http.StatusBadRequest)
			cancel()
			return
		}
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if code == "" {
			http.Error(w, "OAuth callback did not include a code", http.StatusBadRequest)
			return
		}
		select {
		case codeCh <- code:
			entry.finish("verifying", nil)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>Awaken</title><p>%s</p>", html.EscapeString("Authorization received. You can return to Awaken."))
		default:
			http.Error(w, "OAuth callback was already consumed", http.StatusConflict)
		}
	})
	entry.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	s.putSession(entry)
	go func() { _ = entry.server.Serve(listener) }()
	go func() {
		defer func() {
			_ = entry.server.Close()
			cancel()
			s.scheduleSessionRemoval(entry)
		}()
		select {
		case code := <-codeCh:
			exchangeOptions := []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("code_verifier", verifier)}
			if requirement.OAuth.Resource != "" {
				exchangeOptions = append(exchangeOptions, oauth2.SetAuthURLParam("resource", requirement.OAuth.Resource))
			}
			clientCtx := context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
			token, exchangeErr := config.Exchange(clientCtx, code, exchangeOptions...)
			if exchangeErr != nil {
				entry.finish("failed", fmt.Errorf("exchange OAuth code: %w", exchangeErr))
				return
			}
			stored := mcpclient.StoredTokenSource{Token: token, AuthURL: authURL, TokenURL: tokenURL, RedirectURL: callbackURL, ClientID: clientID, ClientSecret: clientSecret, Scopes: append([]string(nil), requirement.OAuth.Scopes...)}
			if saveErr := s.saveCredential(pluginName, requirement, stored); saveErr != nil {
				entry.finish("failed", saveErr)
				return
			}
			entry.finish("ready", nil)
			if s.plugins != nil {
				s.plugins.RefreshSetupReadiness()
			}
			s.activateMCPBinding(pluginName, requirement)
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				entry.finish("expired", errors.New("authorization expired"))
			} else if entry.view().Status != "failed" {
				entry.finish("cancelled", errors.New("authorization cancelled"))
			}
		}
	}()
	return StartResult{SessionID: entry.id, Status: "authorizing", AuthorizationURL: authorizationURL, ExpiresAt: &entry.expiresAt}, nil
}

type deviceAuthorizationResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type oauthErrorResponse struct {
	Error string `json:"error"`
}

func (s *Service) startDevice(pluginName string, requirement ir.SetupRequirement) (StartResult, error) {
	clientID, clientSecret, err := s.oauthClient(pluginName, requirement)
	if err != nil {
		return StartResult{}, err
	}
	spec := requirement.OAuth
	if err := ValidateActionURL(spec.DeviceAuthorizationEndpoint); err != nil {
		return StartResult{}, err
	}
	if err := ValidateActionURL(spec.TokenEndpoint); err != nil {
		return StartResult{}, err
	}
	form := url.Values{"client_id": {clientID}}
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	if len(spec.Scopes) > 0 {
		form.Set("scope", strings.Join(spec.Scopes, " "))
	}
	if spec.Resource != "" {
		form.Set("resource", spec.Resource)
	}
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, spec.DeviceAuthorizationEndpoint, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.httpClient.Do(request)
	if err != nil {
		return StartResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return StartResult{}, fmt.Errorf("device authorization returned HTTP %d", response.StatusCode)
	}
	var device deviceAuthorizationResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&device); err != nil {
		return StartResult{}, err
	}
	if device.DeviceCode == "" || device.UserCode == "" || device.VerificationURI == "" {
		return StartResult{}, errors.New("device authorization response is incomplete")
	}
	displayUserCode := device.VerificationURIComplete == ""
	verificationURL := device.VerificationURIComplete
	if verificationURL == "" {
		verificationURL = device.VerificationURI
	}
	if err := ValidateActionURL(verificationURL); err != nil {
		return StartResult{}, err
	}
	expiresIn := time.Duration(device.ExpiresIn) * time.Second
	if expiresIn <= 0 || expiresIn > sessionTTL {
		expiresIn = sessionTTL
	}
	ctx, cancel := context.WithTimeout(context.Background(), expiresIn)
	entry := &session{id: uuid.NewString(), pluginName: pluginName, setupID: requirement.ID, fingerprint: requirement.Fingerprint, identity: pluginCredentialIdentity(pluginName, requirement), status: "authorizing", expiresAt: time.Now().UTC().Add(expiresIn), cancel: cancel}
	s.putSession(entry)
	interval := time.Duration(device.Interval) * time.Second
	if interval < s.deviceMinimumInterval {
		interval = s.deviceMinimumInterval
	}
	go s.pollDevice(ctx, entry, requirement, clientID, clientSecret, device.DeviceCode, interval)
	return StartResult{SessionID: entry.id, Status: "authorizing", VerificationURL: verificationURL, UserCode: device.UserCode, DisplayUserCode: displayUserCode, ExpiresAt: &entry.expiresAt}, nil
}

func (s *Service) pollDevice(ctx context.Context, entry *session, requirement ir.SetupRequirement, clientID, clientSecret, deviceCode string, interval time.Duration) {
	defer func() {
		entry.cancel()
		s.scheduleSessionRemoval(entry)
	}()
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				entry.finish("expired", errors.New("device authorization expired"))
			} else if entry.view().Status != "failed" {
				entry.finish("cancelled", errors.New("device authorization cancelled"))
			}
			return
		case <-time.After(interval):
		}
		form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {deviceCode}, "client_id": {clientID}}
		if clientSecret != "" {
			form.Set("client_secret", clientSecret)
		}
		request, _ := http.NewRequestWithContext(ctx, http.MethodPost, requirement.OAuth.TokenEndpoint, strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response, err := s.httpClient.Do(request)
		if err != nil {
			entry.finish("failed", err)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		_ = response.Body.Close()
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			var payload struct {
				AccessToken  string `json:"access_token"`
				TokenType    string `json:"token_type"`
				RefreshToken string `json:"refresh_token"`
				ExpiresIn    int    `json:"expires_in"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				entry.finish("failed", err)
				return
			}
			if payload.AccessToken == "" {
				entry.finish("failed", errors.New("device token response omitted access_token"))
				return
			}
			token := &oauth2.Token{AccessToken: payload.AccessToken, TokenType: payload.TokenType, RefreshToken: payload.RefreshToken}
			if payload.ExpiresIn > 0 {
				token.Expiry = time.Now().UTC().Add(time.Duration(payload.ExpiresIn) * time.Second)
			}
			stored := mcpclient.StoredTokenSource{Token: token, TokenURL: requirement.OAuth.TokenEndpoint, ClientID: clientID, ClientSecret: clientSecret, Scopes: append([]string(nil), requirement.OAuth.Scopes...)}
			if err := s.saveCredential(entry.pluginName, requirement, stored); err != nil {
				entry.finish("failed", err)
				return
			}
			entry.finish("ready", nil)
			if s.plugins != nil {
				s.plugins.RefreshSetupReadiness()
			}
			s.activateMCPBinding(entry.pluginName, requirement)
			return
		}
		var oauthErr oauthErrorResponse
		_ = json.Unmarshal(body, &oauthErr)
		switch oauthErr.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "expired_token":
			entry.finish("expired", errors.New("device authorization expired"))
		default:
			entry.finish("failed", fmt.Errorf("device token request failed: %s", firstNonEmpty(oauthErr.Error, strconv.Itoa(response.StatusCode))))
		}
		return
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "unknown"
}

// ValidateActionURL is also used by Electron Main before opening a browser.
// Public services must use HTTPS; HTTP is accepted only for loopback test and
// local-service flows.
func ValidateActionURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("authorization URL is invalid")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("authorization URL must use HTTPS")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	loopback := host == "localhost"
	if ip := net.ParseIP(host); ip != nil {
		loopback = ip.IsLoopback()
		if !loopback && (ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast()) {
			return errors.New("authorization URL resolves to a private or local address")
		}
	}
	if parsed.Scheme == "http" && !loopback {
		return errors.New("authorization URL must use public HTTPS or loopback HTTP")
	}
	for key := range parsed.Query() {
		switch strings.ToLower(key) {
		case "access_token", "token", "client_secret", "password", "api_key":
			return errors.New("authorization URL contains a credential query parameter")
		}
	}
	return nil
}

type safeOAuthRoundTripper struct{ transport *http.Transport }

func (r safeOAuthRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := ValidateActionURL(request.URL.String()); err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(request.Context(), request.URL.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("OAuth endpoint DNS lookup failed")
	}
	for _, candidate := range addresses {
		ip := candidate.IP
		if !ip.IsLoopback() && (ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast()) {
			return nil, errors.New("OAuth endpoint DNS resolved to a blocked address")
		}
	}
	return r.transport.RoundTrip(request)
}

func newSafeHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		var allowed net.IP
		for _, candidate := range addresses {
			ip := candidate.IP
			if ip.IsLoopback() || (!ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() && !ip.IsMulticast()) {
				allowed = ip
				break
			}
		}
		if allowed == nil {
			return nil, errors.New("OAuth endpoint did not resolve to an allowed address")
		}
		return (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, net.JoinHostPort(allowed.String(), port))
	}
	return &http.Client{Transport: safeOAuthRoundTripper{transport: transport}, Timeout: 30 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many OAuth redirects")
		}
		return ValidateActionURL(request.URL.String())
	}}
}
