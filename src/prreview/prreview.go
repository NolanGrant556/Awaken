package prreview

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

type CommentsInput struct {
	PRComments     []Comment
	ReviewComments []Comment
}

type Comment struct {
	ID          string
	InReplyToID string
	Author      string
	Body        string
	Created     time.Time
	Path        string
	Line        int
	DiffHunk    string
}

type Thread struct {
	Comment Comment
	Replies []Thread
}

func FormatComments(input CommentsInput) string {
	comments := input.comments()
	if len(comments) == 0 {
		return "No comments found."
	}

	threads := ThreadComments(comments)
	var builder strings.Builder
	builder.WriteString("## Comments\n\n")
	for i, thread := range threads {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		writeThread(&builder, thread, 0)
	}

	return strings.TrimRight(builder.String(), "\n")
}

func ThreadComments(comments []Comment) []Thread {
	sorted := append([]Comment(nil), comments...)
	sortComments(sorted)

	nodes := make([]*threadNode, 0, len(sorted))
	byID := make(map[string]*threadNode, len(sorted))
	for _, comment := range sorted {
		node := &threadNode{comment: comment}
		nodes = append(nodes, node)
		if comment.ID != "" {
			if _, exists := byID[comment.ID]; !exists {
				byID[comment.ID] = node
			}
		}
	}

	roots := make([]*threadNode, 0, len(nodes))
	for _, node := range nodes {
		parentID := node.comment.InReplyToID
		if parentID == "" {
			roots = append(roots, node)
			continue
		}

		parent, ok := byID[parentID]
		if !ok || parent == node || parent.comment.InReplyToID == node.comment.ID {
			roots = append(roots, node)
			continue
		}

		parent.replies = append(parent.replies, node)
	}

	threads := make([]Thread, 0, len(roots))
	for _, root := range roots {
		threads = append(threads, root.thread())
	}
	return threads
}

func BuildReviewPrompt(args string) string {
	prNumber := strings.TrimSpace(args)
	return strings.TrimSpace("You are an expert code reviewer. Follow these steps:\n\n" +
		"1. If no PR number is provided in the args, run `gh pr list` to show open PRs.\n" +
		"2. If a PR number is provided, run `gh pr view <number>` to get PR details.\n" +
		"3. Run `gh pr diff <number>` to get the diff.\n" +
		"4. Analyze the changes and provide a concise, thorough code review.\n\n" +
		"Focus on:\n" +
		"- Correctness\n" +
		"- Project conventions\n" +
		"- Performance implications\n" +
		"- Tests and coverage\n" +
		"- Security considerations\n\n" +
		"Format the review with clear sections and bullet points.\n\n" +
		"PR number: " + prNumber)
}

type threadNode struct {
	comment Comment
	replies []*threadNode
}

func (input CommentsInput) comments() []Comment {
	comments := make([]Comment, 0, len(input.PRComments)+len(input.ReviewComments))
	comments = append(comments, input.PRComments...)
	comments = append(comments, input.ReviewComments...)
	return comments
}

func (node *threadNode) thread() Thread {
	thread := Thread{Comment: node.comment}
	if len(node.replies) == 0 {
		return thread
	}

	thread.Replies = make([]Thread, 0, len(node.replies))
	for _, reply := range node.replies {
		thread.Replies = append(thread.Replies, reply.thread())
	}
	return thread
}

func sortComments(comments []Comment) {
	sort.SliceStable(comments, func(i, j int) bool {
		return compareComments(comments[i], comments[j]) < 0
	})
}

func compareComments(a, b Comment) int {
	if a.Path != b.Path {
		return strings.Compare(a.Path, b.Path)
	}
	if a.Line != b.Line {
		if a.Line < b.Line {
			return -1
		}
		return 1
	}
	if !a.Created.Equal(b.Created) {
		if a.Created.Before(b.Created) {
			return -1
		}
		return 1
	}
	return strings.Compare(a.ID, b.ID)
}

func writeThread(builder *strings.Builder, thread Thread, depth int) {
	itemIndent := strings.Repeat("  ", depth)
	bodyIndent := strings.Repeat("  ", depth+1)

	builder.WriteString(itemIndent)
	builder.WriteString("- ")
	builder.WriteString(formatAuthor(thread.Comment.Author))
	builder.WriteByte(' ')
	builder.WriteString(formatLocation(thread.Comment))
	builder.WriteString(":\n")

	writeDiffHunk(builder, bodyIndent, thread.Comment.DiffHunk)
	writeQuotedBody(builder, bodyIndent, thread.Comment.Body)

	for _, reply := range thread.Replies {
		builder.WriteString("\n\n")
		writeThread(builder, reply, depth+1)
	}
}

func writeDiffHunk(builder *strings.Builder, indent string, diffHunk string) {
	diffHunk = strings.Trim(diffHunk, "\n")
	if diffHunk == "" {
		return
	}

	builder.WriteString(indent)
	builder.WriteString("```diff\n")
	for _, line := range strings.Split(diffHunk, "\n") {
		builder.WriteString(indent)
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	builder.WriteString(indent)
	builder.WriteString("```\n")
}

func writeQuotedBody(builder *strings.Builder, indent string, body string) {
	body = strings.Trim(body, "\n")
	if body == "" {
		builder.WriteString(indent)
		builder.WriteString("> ")
		return
	}

	for i, line := range strings.Split(body, "\n") {
		if i > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(indent)
		builder.WriteString("> ")
		builder.WriteString(line)
	}
}

func formatAuthor(author string) string {
	author = strings.TrimSpace(author)
	if author == "" {
		return "@unknown"
	}
	if strings.HasPrefix(author, "@") {
		return author
	}
	return "@" + author
}

func formatLocation(comment Comment) string {
	path := strings.TrimSpace(comment.Path)
	switch {
	case path != "" && comment.Line > 0:
		return path + "#" + strconv.Itoa(comment.Line)
	case path != "":
		return path
	case comment.Line > 0:
		return "#" + strconv.Itoa(comment.Line)
	default:
		return "PR"
	}
}
