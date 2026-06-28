package remotesse

import (
	"net/url"
	"strconv"
)

const (
	defaultSeenLimit  = 1000
	defaultSeenWindow = int64(200)
)

// Cursor tracks SSE sequence numbers for duplicate suppression and resumption.
type Cursor struct {
	lastSequenceNum int64
	seen            map[int64]struct{}
	order           []int64
	seenLimit       int
	seenWindow      int64
}

// NewCursor returns a cursor seeded with an optional high-water sequence.
func NewCursor(lastSequenceNum int64) *Cursor {
	c := &Cursor{lastSequenceNum: lastSequenceNum}
	c.ensureDefaults()
	return c
}

// LastSequenceNum returns the highest accepted sequence number.
func (c *Cursor) LastSequenceNum() int64 {
	if c == nil {
		return 0
	}
	return c.lastSequenceNum
}

// Accept records seq and reports whether it has not been seen recently.
//
// Out-of-order sequence numbers are accepted when they are not in the bounded
// seen set, but they do not lower the high-water mark.
func (c *Cursor) Accept(seq int64) bool {
	if c == nil || seq <= 0 {
		return false
	}
	c.ensureDefaults()

	if _, ok := c.seen[seq]; ok {
		return false
	}

	c.seen[seq] = struct{}{}
	c.order = append(c.order, seq)
	if seq > c.lastSequenceNum {
		c.lastSequenceNum = seq
	}
	c.pruneSeen()
	return true
}

// SeenCount returns the number of sequence numbers retained for dedupe.
func (c *Cursor) SeenCount() int {
	if c == nil {
		return 0
	}
	return len(c.seen)
}

// ResumeQuery returns the query parameters needed to resume from the cursor.
func (c *Cursor) ResumeQuery() url.Values {
	values := url.Values{}
	last := c.LastSequenceNum()
	if last > 0 {
		values.Set("from_sequence_num", strconv.FormatInt(last, 10))
	}
	return values
}

// ApplyResume adds resume query parameters to rawURL when the cursor has state.
func (c *Cursor) ApplyResume(rawURL string) (string, error) {
	last := c.LastSequenceNum()
	if last <= 0 {
		return rawURL, nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("from_sequence_num", strconv.FormatInt(last, 10))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// Headers returns resume headers for an SSE request.
func (c *Cursor) Headers() map[string]string {
	headers := map[string]string{}
	last := c.LastSequenceNum()
	if last > 0 {
		headers["Last-Event-ID"] = strconv.FormatInt(last, 10)
	}
	return headers
}

func (c *Cursor) ensureDefaults() {
	if c.seen == nil {
		c.seen = make(map[int64]struct{})
	}
	if c.seenLimit <= 0 {
		c.seenLimit = defaultSeenLimit
	}
	if c.seenWindow <= 0 {
		c.seenWindow = defaultSeenWindow
	}
}

func (c *Cursor) pruneSeen() {
	if len(c.seen) <= c.seenLimit {
		return
	}

	threshold := c.lastSequenceNum - c.seenWindow
	if threshold > 0 {
		for _, seq := range c.order {
			if seq < threshold {
				delete(c.seen, seq)
			}
		}
	}

	for _, seq := range c.order {
		if len(c.seen) <= c.seenLimit {
			break
		}
		delete(c.seen, seq)
	}

	c.compactOrder()
}

func (c *Cursor) compactOrder() {
	n := 0
	for _, seq := range c.order {
		if _, ok := c.seen[seq]; ok {
			c.order[n] = seq
			n++
		}
	}
	c.order = c.order[:n]
}
