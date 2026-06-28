// Package urlsafety evaluates URL targets for SSRF-sensitive fetch paths.
//
// The package is intentionally pure: it parses the URL and evaluates the IPs
// supplied by the caller, but it never performs DNS lookups or network I/O.
// Callers are responsible for resolving the URL host immediately before the
// request and passing every resolved address into Evaluate.
//
// This policy blocks cloud metadata endpoints, link-local addresses, loopback
// addresses, private addresses by default, reserved ranges, multicast,
// unspecified addresses, and CGNAT shared address space. AllowPrivateURLs only
// relaxes ordinary private address blocking. It does not relax metadata,
// link-local, loopback, reserved, multicast, unspecified, or CGNAT blocking.
//
// Limitations:
//   - DNS rebinding is a time-of-check/time-of-use problem. A preflight
//     decision cannot prove that the HTTP transport will connect to the same
//     address later. Callers that need stronger protection should re-check the
//     peer address at connection time or send requests through an egress proxy.
//   - Redirect targets are separate requests. Callers must resolve and evaluate
//     every redirect location before following it; a Decision for the original
//     URL does not apply to a redirected URL.
package urlsafety
