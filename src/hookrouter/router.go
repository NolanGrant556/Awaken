package hookrouter

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
)

type MatcherMode string

const (
	MatcherModeAll     MatcherMode = "all"
	MatcherModeRegex   MatcherMode = "regex"
	MatcherModeLiteral MatcherMode = "literal"
)

var (
	ErrInvalidHook   = errors.New("invalid hook")
	ErrDuplicateHook = errors.New("hook already registered")
)

type Hook struct {
	ID          string
	Event       string
	Matcher     string
	MatcherMode MatcherMode
	PluginName  string
	SkillName   string
	Command     string
	Managed     bool
}

type RouteRequest struct {
	Event      string
	ToolName   string
	PluginName string
	SkillName  string
}

type Router struct {
	mu    sync.RWMutex
	hooks []routeHook
	byID  map[string]struct{}
}

type routeHook struct {
	hook    Hook
	matcher *regexp.Regexp
}

func NewRouter() *Router {
	return &Router{
		byID: make(map[string]struct{}),
	}
}

func (r *Router) Add(hook Hook) error {
	if hook.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidHook)
	}
	if hook.Event == "" {
		return fmt.Errorf("%w: event is required", ErrInvalidHook)
	}

	route := routeHook{hook: hook}
	route.hook.MatcherMode = MatcherModeAll
	if hook.Matcher != "" {
		matcher, err := regexp.Compile(hook.Matcher)
		if err != nil {
			route.hook.MatcherMode = MatcherModeLiteral
		} else {
			matcher.Longest()
			route.matcher = matcher
			route.hook.MatcherMode = MatcherModeRegex
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.byID == nil {
		r.byID = make(map[string]struct{})
	}
	if _, exists := r.byID[hook.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateHook, hook.ID)
	}

	r.hooks = append(r.hooks, route)
	r.byID[hook.ID] = struct{}{}
	return nil
}

func (r *Router) Routes(req RouteRequest) []Hook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make([]Hook, 0, len(r.hooks))
	for _, route := range r.hooks {
		if route.hook.Event != req.Event {
			continue
		}
		if route.hook.PluginName != "" && route.hook.PluginName != req.PluginName {
			continue
		}
		if route.hook.SkillName != "" && route.hook.SkillName != req.SkillName {
			continue
		}
		if !route.matchesTool(req.ToolName) {
			continue
		}
		routes = append(routes, route.hook)
	}
	return routes
}

func (r *Router) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, route := range r.hooks {
		if route.hook.ID != id {
			continue
		}

		copy(r.hooks[i:], r.hooks[i+1:])
		r.hooks[len(r.hooks)-1] = routeHook{}
		r.hooks = r.hooks[:len(r.hooks)-1]
		if r.byID != nil {
			delete(r.byID, id)
		}
		return
	}
}

func (r *Router) Snapshot() []Hook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hooks := make([]Hook, 0, len(r.hooks))
	for _, route := range r.hooks {
		hooks = append(hooks, route.hook)
	}
	return hooks
}

func (r routeHook) matchesTool(toolName string) bool {
	switch r.hook.MatcherMode {
	case MatcherModeAll:
		return true
	case MatcherModeRegex:
		return regexpFullMatch(r.matcher, toolName)
	case MatcherModeLiteral:
		return toolName == r.hook.Matcher
	default:
		return false
	}
}

func regexpFullMatch(matcher *regexp.Regexp, value string) bool {
	if matcher == nil {
		return false
	}
	match := matcher.FindStringIndex(value)
	return match != nil && match[0] == 0 && match[1] == len(value)
}
