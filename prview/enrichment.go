package prview

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type CheckCounts struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Pending int `json:"pending"`
	Skipped int `json:"skipped"`
}
type Stack struct {
	Position int `json:"position"`
	Total    int `json:"total"`
}
type Summary struct {
	Repository string      `json:"repository"`
	Number     int         `json:"number"`
	Review     string      `json:"review"`
	Checks     CheckCounts `json:"checks"`
	Comments   int         `json:"comments"`
	Assignees  []string    `json:"assignees"`
	Stack      *Stack      `json:"stack"`
	Error      string      `json:"error,omitempty"`
}
type stateCount struct {
	State string
	Count int
}
type graphPR struct {
	Comments          struct{ TotalCount int }
	ReviewDecision    string
	HeadRefOid        string
	Assignees         struct{ Nodes []struct{ Login string } }
	Stack             *struct{ Size int }
	StackEntry        *struct{ Position int }
	StatusCheckRollup *struct {
		Contexts struct {
			TotalCount                 int
			CheckRunCountsByState      []stateCount
			StatusContextCountsByState []stateCount
			Nodes                      []struct {
				Typename                                                                                               string `json:"__typename"`
				ID, Name, Context, Status, Conclusion, State, DetailsURL, TargetURL, StartedAt, CreatedAt, Description string
				CheckSuite                                                                                             *struct{ App *struct{ Slug string } }
			}
			PageInfo struct {
				HasNextPage bool
				EndCursor   string
			}
		}
	}
}

type partialGraphError struct{ aliases map[string]bool }

func (e *partialGraphError) Error() string { return "GitHub could not load some requested data" }
func (s *Service) graph(ctx context.Context, query string, out any) error {
	b, _ := json.Marshal(map[string]string{"query": query})
	var response struct {
		Data   json.RawMessage
		Errors []struct{ Path []any }
	}
	if err := s.request(ctx, "POST", "/graphql", b, &response); err != nil {
		return err
	}
	if len(response.Data) == 0 || string(response.Data) == "null" {
		return &APIError{Status: 502, Code: "invalid_github_response", Message: "GitHub returned no data"}
	}
	if json.Unmarshal(response.Data, out) != nil {
		return &APIError{Status: 502, Code: "invalid_github_response", Message: "GitHub returned an unreadable response"}
	}
	if len(response.Errors) > 0 {
		aliases := map[string]bool{}
		for _, e := range response.Errors {
			if len(e.Path) == 0 {
				return &APIError{Status: 502, Code: "github_graphql_error", Message: "GitHub could not load the requested data"}
			}
			alias, ok := e.Path[0].(string)
			if !ok {
				return &APIError{Status: 502, Code: "github_graphql_error", Message: "GitHub could not load the requested data"}
			}
			aliases[alias] = true
		}
		return &partialGraphError{aliases}
	}
	return nil
}
func parseRef(ref string) (string, int, error) {
	p := strings.Split(ref, "/")
	if len(p) != 3 || !repository.MatchString(p[0]+"/"+p[1]) || p[1] == "." || p[1] == ".." {
		return "", 0, bad("Invalid pull request reference")
	}
	n, e := strconv.Atoi(p[2])
	if e != nil || n < 1 || n > 2147483647 {
		return "", 0, bad("Invalid pull request number")
	}
	return p[0] + "/" + p[1], n, nil
}
func countChecks(p graphPR) CheckCounts {
	c := CheckCounts{}
	if p.StatusCheckRollup == nil {
		return c
	}
	x := p.StatusCheckRollup.Contexts
	c.Total = x.TotalCount
	for _, groups := range [][]stateCount{x.CheckRunCountsByState, x.StatusContextCountsByState} {
		for _, v := range groups {
			switch v.State {
			case "SUCCESS":
				c.Passed += v.Count
			case "FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE":
				c.Failed += v.Count
			case "NEUTRAL", "SKIPPED":
				c.Skipped += v.Count
			default:
				c.Pending += v.Count
			}
		}
	}
	return c
}

const summaryFields = `reviewDecision comments{totalCount} assignees(first:100){nodes{login}} stack{size} stackEntry{position} statusCheckRollup{contexts{totalCount checkRunCountsByState{state count} statusContextCountsByState{state count}}}`

func (s *Service) Summaries(ctx context.Context, v url.Values) (any, error) {
	if len(v) != 1 || len(v["pr"]) < 1 || len(v["pr"]) > 30 {
		return nil, bad("Provide between 1 and 30 pr references")
	}
	items := []Summary{}
	q := "query {"
	seen := map[string]bool{}
	for _, ref := range v["pr"] {
		repo, n, e := parseRef(ref)
		if e != nil {
			return nil, e
		}
		key := strings.ToLower(ref)
		if seen[key] {
			return nil, bad("Duplicate pull request reference")
		}
		seen[key] = true
		p := strings.Split(repo, "/")
		i := len(items)
		q += fmt.Sprintf("p%d:repository(owner:%q,name:%q){pullRequest(number:%d){%s}}", i, p[0], p[1], n, summaryFields)
		items = append(items, Summary{Repository: repo, Number: n, Review: "UNKNOWN", Assignees: []string{}})
	}
	q += "}"
	var data map[string]*struct{ PullRequest *graphPR }
	failed := map[string]bool{}
	if e := s.graph(ctx, q, &data); e != nil {
		if p, ok := e.(*partialGraphError); ok {
			failed = p.aliases
		} else {
			return nil, e
		}
	}
	for i := range items {
		r := data[fmt.Sprintf("p%d", i)]
		if failed[fmt.Sprintf("p%d", i)] || r == nil || r.PullRequest == nil {
			items[i].Error = "not_found_or_inaccessible"
			continue
		}
		p := *r.PullRequest
		items[i].Review = p.ReviewDecision
		if items[i].Review == "" {
			items[i].Review = "NONE"
		}
		items[i].Comments = p.Comments.TotalCount
		items[i].Checks = countChecks(p)
		for _, a := range p.Assignees.Nodes {
			items[i].Assignees = append(items[i].Assignees, a.Login)
		}
		if p.Stack != nil && p.StackEntry != nil {
			items[i].Stack = &Stack{p.StackEntry.Position, p.Stack.Size}
		}
	}
	return map[string]any{"items": items}, nil
}

type DetailItem struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Bot       bool   `json:"bot"`
	Body      string `json:"body"`
	State     string `json:"state"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
	Path      string `json:"path"`
}
type DetailPage struct {
	Items      []DetailItem `json:"items"`
	HasNext    bool         `json:"has_next"`
	NextCursor string       `json:"next_cursor,omitempty"`
	Head       string       `json:"head,omitempty"`
}

func (s *Service) Section(ctx context.Context, owner, repo, number, section string, v url.Values) (DetailPage, error) {
	result := DetailPage{Items: []DetailItem{}}
	_, n, e := parseRef(owner + "/" + repo + "/" + number)
	if e != nil {
		return result, e
	}
	if section == "checks" {
		return s.checks(ctx, owner, repo, n, v)
	}
	paths := map[string]string{"reviews": "pulls/%d/reviews", "comments": "issues/%d/comments", "code-comments": "pulls/%d/comments"}
	path, ok := paths[section]
	if !ok {
		return result, bad("Unknown detail section")
	}
	for k, x := range v {
		if k != "page" || len(x) != 1 {
			return result, bad("Unsupported detail parameter")
		}
	}
	page := 1
	if v.Get("page") != "" {
		page, e = strconv.Atoi(v.Get("page"))
		if e != nil || page < 1 || page > 10000 {
			return result, bad("Invalid page")
		}
	}
	// GitHub REST uses snake_case for these fields.
	var raw []json.RawMessage
	var headers http.Header
	values := url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}}
	if e = s.request(ctx, "GET", "/repos/"+owner+"/"+repo+"/"+fmt.Sprintf(path, n)+"?"+values.Encode(), nil, &raw, &headers); e != nil {
		return result, e
	}
	for _, b := range raw {
		var r struct {
			ID                int64
			Body, State, Path string
			HTMLURL           string `json:"html_url"`
			SubmittedAt       string `json:"submitted_at"`
			CreatedAt         string `json:"created_at"`
			User              struct{ Login, Type string }
		}
		if json.Unmarshal(b, &r) != nil {
			return result, &APIError{Status: 502, Code: "invalid_github_response", Message: "GitHub returned an unreadable response"}
		}
		at := r.CreatedAt
		if at == "" {
			at = r.SubmittedAt
		}
		result.Items = append(result.Items, DetailItem{ID: strconv.FormatInt(r.ID, 10), Author: r.User.Login, Bot: r.User.Type == "Bot" || strings.HasSuffix(r.User.Login, "[bot]"), Body: r.Body, State: r.State, URL: r.HTMLURL, CreatedAt: at, Path: r.Path})
	}
	result.HasNext = strings.Contains(headers.Get("Link"), `rel="next"`)
	return result, nil
}
func (s *Service) checks(ctx context.Context, owner, repo string, n int, v url.Values) (DetailPage, error) {
	result := DetailPage{Items: []DetailItem{}}
	for k, x := range v {
		if (k != "cursor" && k != "head" && k != "page") || len(x) != 1 {
			return result, bad("Unsupported checks parameter")
		}
	}
	if page := v.Get("page"); page != "" {
		n, e := strconv.Atoi(page)
		if e != nil || n < 1 || n > 10000 {
			return result, bad("Invalid page")
		}
	}
	cursor, head := v.Get("cursor"), v.Get("head")
	if len(cursor) > 1024 || strings.ContainsAny(cursor, "\x00\r\n") || (head != "" && (len(head) != 40 || strings.Trim(head, "0123456789abcdefABCDEF") != "")) || (cursor != "" && head == "") {
		return result, bad("Invalid checks cursor or head")
	}
	after := ""
	if cursor != "" {
		after = ",after:" + strconv.Quote(cursor)
	}
	query := fmt.Sprintf(`query{repository(owner:%q,name:%q){pullRequest(number:%d){headRefOid statusCheckRollup{contexts(first:100%s){nodes{__typename ... on CheckRun{id name status conclusion detailsUrl startedAt checkSuite{app{slug}}} ... on StatusContext{id context state targetUrl createdAt description}} pageInfo{hasNextPage endCursor}}}}}}`, owner, repo, n, after)
	var d struct {
		Repository *struct{ PullRequest *graphPR }
	}
	if e := s.graph(ctx, query, &d); e != nil {
		return result, e
	}
	if d.Repository == nil || d.Repository.PullRequest == nil {
		return result, &APIError{Status: 404, Code: "not_found", Message: "Pull request not found or not accessible"}
	}
	p := *d.Repository.PullRequest
	result.Head = p.HeadRefOid
	if head != "" && head != result.Head {
		return result, &APIError{Status: 409, Code: "head_changed", Message: "The pull request head changed. Reload checks."}
	}
	if p.StatusCheckRollup == nil {
		return result, nil
	}
	x := p.StatusCheckRollup.Contexts
	result.HasNext = x.PageInfo.HasNextPage
	result.NextCursor = x.PageInfo.EndCursor
	for _, r := range x.Nodes {
		item := DetailItem{ID: r.ID, Name: r.Name, State: r.Status, URL: r.DetailsURL, CreatedAt: r.StartedAt}
		if r.Typename == "CheckRun" {
			if r.Conclusion != "" {
				item.State = r.Conclusion
			}
			if r.CheckSuite != nil && r.CheckSuite.App != nil {
				item.Author = r.CheckSuite.App.Slug
				item.Bot = true
			}
		} else {
			item.Name = r.Context
			item.State = r.State
			item.URL = r.TargetURL
			item.CreatedAt = r.CreatedAt
			item.Body = r.Description
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}
