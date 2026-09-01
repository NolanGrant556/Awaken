package outlook

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type MailFolder struct {
	ID               string `json:"id"`
	DisplayName      string `json:"displayName"`
	ParentFolderID   string `json:"parentFolderId,omitempty"`
	ChildFolderCount int    `json:"childFolderCount,omitempty"`
	UnreadItemCount  int    `json:"unreadItemCount,omitempty"`
	TotalItemCount   int    `json:"totalItemCount,omitempty"`
	IsHidden         bool   `json:"isHidden,omitempty"`
}

type MailFolderList struct {
	Folders  []MailFolder `json:"folders"`
	NextLink string       `json:"nextLink,omitempty"`
}

type graphMailFolderList struct {
	Value    []MailFolder `json:"value"`
	NextLink string       `json:"@odata.nextLink"`
}

type MailFolderMatch struct {
	ID             string `json:"id"`
	DisplayName    string `json:"displayName"`
	Path           string `json:"path"`
	ParentFolderID string `json:"parentFolderId,omitempty"`
	IsHidden       bool   `json:"isHidden,omitempty"`
	MatchKind      string `json:"matchKind"`
	matchScore     int
}

type MailFolderMatches struct {
	Matches   []MailFolderMatch `json:"matches"`
	Scanned   int               `json:"scanned"`
	Truncated bool              `json:"truncated"`
}

type mailFolderQueueItem struct {
	parentID   string
	parentPath string
}

func (c *Client) ListMailFolders(ctx context.Context, mailbox, parentFolderID string, top int, includeHidden bool) (MailFolderList, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return MailFolderList{}, err
	}
	if top <= 0 {
		top = 50
	}
	if top > 200 {
		top = 200
	}
	path := root + "/mailFolders"
	if strings.TrimSpace(parentFolderID) != "" {
		parentFolderID, err = requiredIdentifier("parentFolderId", parentFolderID)
		if err != nil {
			return MailFolderList{}, err
		}
		path += "/" + url.PathEscape(parentFolderID) + "/childFolders"
	}
	values := url.Values{}
	values.Set("$top", strconv.Itoa(top))
	values.Set("$select", "id,displayName,parentFolderId,childFolderCount,unreadItemCount,totalItemCount,isHidden")
	if includeHidden {
		values.Set("includeHiddenFolders", "true")
	}
	var response graphMailFolderList
	if err := c.do(ctx, http.MethodGet, path+"?"+values.Encode(), nil, &response); err != nil {
		return MailFolderList{}, err
	}
	return MailFolderList{Folders: response.Value, NextLink: response.NextLink}, nil
}

func (c *Client) FindMailFolders(ctx context.Context, mailbox, query string, maxResults int, includeHidden bool) (MailFolderMatches, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return MailFolderMatches{}, err
	}
	query = normalizeFolderPath(query)
	if query == "" {
		return MailFolderMatches{}, fmt.Errorf("query is required")
	}
	if wellKnownMailFolderNames[query] {
		var folder MailFolder
		values := url.Values{}
		values.Set("$select", "id,displayName,parentFolderId,childFolderCount,unreadItemCount,totalItemCount,isHidden")
		if err := c.do(ctx, http.MethodGet, root+"/mailFolders/"+url.PathEscape(query)+"?"+values.Encode(), nil, &folder); err != nil {
			return MailFolderMatches{}, err
		}
		return MailFolderMatches{
			Matches: []MailFolderMatch{{
				ID: folder.ID, DisplayName: folder.DisplayName, Path: folder.DisplayName,
				ParentFolderID: folder.ParentFolderID, IsHidden: folder.IsHidden, MatchKind: "well_known",
			}},
			Scanned: 1,
		}, nil
	}
	if maxResults <= 0 {
		maxResults = 10
	}
	if maxResults > 20 {
		maxResults = 20
	}
	const traversalLimit = 500
	queue := []mailFolderQueueItem{{}}
	visited := make(map[string]struct{})
	result := MailFolderMatches{Matches: []MailFolderMatch{}}
	for len(queue) > 0 && result.Scanned < traversalLimit {
		current := queue[0]
		queue = queue[1:]
		folders, err := c.ListMailFolders(ctx, mailbox, current.parentID, 200, includeHidden)
		if err != nil {
			return MailFolderMatches{}, err
		}
		resourcePath := root + "/mailFolders"
		if current.parentID != "" {
			resourcePath += "/" + url.PathEscape(current.parentID) + "/childFolders"
		}
		for folders.NextLink != "" && result.Scanned+len(folders.Folders) < traversalLimit {
			cursor := base64.RawURLEncoding.EncodeToString([]byte(folders.NextLink))
			continuation, cursorErr := c.continuationPath(cursor, resourcePath)
			if cursorErr != nil {
				return MailFolderMatches{}, cursorErr
			}
			var page graphMailFolderList
			if err := c.do(ctx, http.MethodGet, continuation, nil, &page); err != nil {
				return MailFolderMatches{}, err
			}
			folders.Folders = append(folders.Folders, page.Value...)
			folders.NextLink = page.NextLink
		}
		if folders.NextLink != "" {
			result.Truncated = true
		}
		for _, folder := range folders.Folders {
			if result.Scanned >= traversalLimit {
				result.Truncated = true
				break
			}
			if _, ok := visited[folder.ID]; ok {
				continue
			}
			visited[folder.ID] = struct{}{}
			result.Scanned++
			path := strings.Trim(strings.TrimSpace(current.parentPath)+"/"+strings.TrimSpace(folder.DisplayName), "/")
			if folder.ChildFolderCount > 0 {
				queue = append(queue, mailFolderQueueItem{parentID: folder.ID, parentPath: path})
			}
			kind, score, matched := folderMatch(path, folder.DisplayName, query)
			if !matched {
				continue
			}
			result.Matches = append(result.Matches, MailFolderMatch{
				ID: folder.ID, DisplayName: folder.DisplayName, Path: path,
				ParentFolderID: folder.ParentFolderID, IsHidden: folder.IsHidden,
				MatchKind: kind, matchScore: score,
			})
		}
	}
	if len(queue) > 0 {
		result.Truncated = true
	}
	sort.SliceStable(result.Matches, func(i, j int) bool {
		left, right := result.Matches[i], result.Matches[j]
		if left.matchScore != right.matchScore {
			return left.matchScore < right.matchScore
		}
		leftDepth, rightDepth := strings.Count(left.Path, "/"), strings.Count(right.Path, "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return strings.ToLower(left.Path) < strings.ToLower(right.Path)
	})
	if len(result.Matches) > maxResults {
		result.Matches = result.Matches[:maxResults]
	}
	return result, nil
}

var wellKnownMailFolderNames = map[string]bool{
	"archive": true, "clutter": true, "conflicts": true, "conversationhistory": true,
	"deleteditems": true, "drafts": true, "inbox": true, "junkemail": true,
	"localfailures": true, "msgfolderroot": true, "outbox": true,
	"recoverableitemsdeletions": true, "searchfolders": true, "sentitems": true,
	"serverfailures": true, "syncissues": true,
}

func normalizeFolderPath(value string) string {
	return strings.ToLower(strings.Trim(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"), "/"))
}

func folderMatch(path, displayName, query string) (string, int, bool) {
	normalizedPath := normalizeFolderPath(path)
	normalizedName := strings.ToLower(strings.TrimSpace(displayName))
	switch {
	case normalizedPath == query:
		return "exact_path", 0, true
	case normalizedName == query:
		return "exact_name", 1, true
	case strings.HasSuffix(normalizedPath, "/"+query):
		return "path_suffix", 2, true
	case strings.Contains(normalizedPath, query) || strings.Contains(normalizedName, query):
		return "contains", 3, true
	default:
		return "", 0, false
	}
}

func (c *Client) CreateMailFolder(ctx context.Context, mailbox, parentFolderID, displayName string, hidden bool) (MailFolder, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return MailFolder{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > 255 || strings.ContainsAny(displayName, "\r\n") {
		return MailFolder{}, fmt.Errorf("displayName must contain 1 to 255 characters")
	}
	path := root + "/mailFolders"
	if strings.TrimSpace(parentFolderID) != "" {
		parentFolderID, err = requiredIdentifier("parentFolderId", parentFolderID)
		if err != nil {
			return MailFolder{}, err
		}
		path += "/" + url.PathEscape(parentFolderID) + "/childFolders"
	}
	var folder MailFolder
	if err := c.do(ctx, http.MethodPost, path, map[string]any{"displayName": displayName, "isHidden": hidden}, &folder); err != nil {
		return MailFolder{}, err
	}
	return folder, nil
}

func (c *Client) UpdateMailFolder(ctx context.Context, mailbox, folderID, displayName string) (MailFolder, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return MailFolder{}, err
	}
	folderID, err = requiredIdentifier("folderId", folderID)
	if err != nil {
		return MailFolder{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > 255 || strings.ContainsAny(displayName, "\r\n") {
		return MailFolder{}, fmt.Errorf("displayName must contain 1 to 255 characters")
	}
	var folder MailFolder
	if err := c.do(ctx, http.MethodPatch, root+"/mailFolders/"+url.PathEscape(folderID), map[string]string{"displayName": displayName}, &folder); err != nil {
		return MailFolder{}, err
	}
	return folder, nil
}

func (c *Client) DeleteMailFolder(ctx context.Context, mailbox, folderID string) error {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return err
	}
	folderID, err = requiredIdentifier("folderId", folderID)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodDelete, root+"/mailFolders/"+url.PathEscape(folderID), nil, nil)
}

type OutlookCategory struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Color       string `json:"color"`
}

type CategoryList struct {
	Categories []OutlookCategory `json:"categories"`
}

type graphCategoryList struct {
	Value []OutlookCategory `json:"value"`
}

func (c *Client) ListCategories(ctx context.Context) (CategoryList, error) {
	var response graphCategoryList
	if err := c.do(ctx, http.MethodGet, "/me/outlook/masterCategories?$select=id,displayName,color", nil, &response); err != nil {
		return CategoryList{}, err
	}
	return CategoryList{Categories: response.Value}, nil
}

func (c *Client) CreateCategory(ctx context.Context, displayName, color string) (OutlookCategory, error) {
	payload, err := categoryPayload(displayName, color)
	if err != nil {
		return OutlookCategory{}, err
	}
	var category OutlookCategory
	if err := c.do(ctx, http.MethodPost, "/me/outlook/masterCategories", payload, &category); err != nil {
		return OutlookCategory{}, err
	}
	return category, nil
}

func (c *Client) UpdateCategory(ctx context.Context, categoryID, color string) (OutlookCategory, error) {
	categoryID, err := requiredIdentifier("categoryId", categoryID)
	if err != nil {
		return OutlookCategory{}, err
	}
	color = strings.TrimSpace(color)
	if !validCategoryColor(color) {
		return OutlookCategory{}, fmt.Errorf("color must be none or preset0 through preset24")
	}
	var category OutlookCategory
	if err := c.do(ctx, http.MethodPatch, "/me/outlook/masterCategories/"+url.PathEscape(categoryID), map[string]string{"color": color}, &category); err != nil {
		return OutlookCategory{}, err
	}
	if category.ID != categoryID || category.Color != color {
		return OutlookCategory{}, fmt.Errorf("Microsoft Graph did not persist the requested category color")
	}
	return category, nil
}

func (c *Client) DeleteCategory(ctx context.Context, categoryID string) error {
	categoryID, err := requiredIdentifier("categoryId", categoryID)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodDelete, "/me/outlook/masterCategories/"+url.PathEscape(categoryID), nil, nil)
}

func categoryPayload(displayName, color string) (map[string]string, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > 255 || strings.ContainsAny(displayName, "\r\n") {
		return nil, fmt.Errorf("displayName must contain 1 to 255 characters")
	}
	color = strings.TrimSpace(color)
	if !validCategoryColor(color) {
		return nil, fmt.Errorf("color must be none or preset0 through preset24")
	}
	return map[string]string{"displayName": displayName, "color": color}, nil
}

func validCategoryColor(value string) bool {
	if value == "none" {
		return true
	}
	if !strings.HasPrefix(value, "preset") {
		return false
	}
	number, err := strconv.Atoi(strings.TrimPrefix(value, "preset"))
	return err == nil && number >= 0 && number <= 24
}
