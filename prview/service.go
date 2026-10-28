// Package prview provides read-only GitHub pull request search for beadview.
package prview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Filters struct {
	Q       string   `json:"q"`
	Orgs    []string `json:"org"`
	Repos   []string `json:"repo"`
	Authors []string `json:"author"`
	State   string   `json:"state"`
	Draft   string   `json:"draft"`
	Sort    string   `json:"sort"`
	Order   string   `json:"order"`
	Page    int      `json:"page"`
	PerPage int      `json:"per_page"`
}
type PR struct {
	Number     int    `json:"number"`
	Repository string `json:"repository"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Author     string `json:"author"`
	State      string `json:"state"`
	Draft      bool   `json:"draft"`
	UpdatedAt  string `json:"updated_at"`
	Body       string `json:"body,omitempty"`
	Merged     *bool  `json:"merged,omitempty"`
	Review     string `json:"review"`
	Checks     string `json:"checks"`
}
type Result struct {
	Items      []PR      `json:"items"`
	Total      int       `json:"total_count"`
	Filters    Filters   `json:"applied_filters"`
	Page       int       `json:"page"`
	PerPage    int       `json:"per_page"`
	HasNext    bool      `json:"has_next"`
	Incomplete bool      `json:"incomplete_results"`
	Truncated  bool      `json:"truncated"`
	FetchedAt  time.Time `json:"fetched_at"`
}
type APIError struct {
	Status     int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	RetryAfter string `json:"retry_after,omitempty"`
	Reset      string `json:"rate_limit_reset,omitempty"`
}

func (e *APIError) Error() string { return e.Message }
func bad(message string) error {
	return &APIError{Status: 400, Code: "invalid_filter", Message: message}
}

var login = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)
var authorLogin = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}(\[bot\])?$`)
var repository = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}/[A-Za-z0-9_.-]{1,100}$`)

func Parse(v url.Values) (Filters, error) {
	f := Filters{Q: strings.TrimSpace(v.Get("q")), State: v.Get("state"), Draft: v.Get("draft"), Sort: v.Get("sort"), Order: v.Get("order"), Page: 1, PerPage: 30, Orgs: []string{}, Repos: []string{}, Authors: []string{}}
	allowed := map[string]bool{"q": true, "org": true, "repo": true, "author": true, "state": true, "draft": true, "sort": true, "order": true, "page": true, "per_page": true}
	for key, values := range v {
		if !allowed[key] {
			return f, bad("Unsupported parameter: " + key)
		}
		if key != "org" && key != "repo" && key != "author" && len(values) > 1 {
			return f, bad("Repeated parameter: " + key)
		}
	}
	if f.State == "" {
		f.State = "open"
	}
	if f.Draft == "" {
		f.Draft = "any"
	}
	if f.Sort == "" {
		f.Sort = "updated"
	}
	if f.Order == "" {
		f.Order = "desc"
	}
	if f.State != "open" && f.State != "closed" && f.State != "all" {
		return f, bad("State must be open, closed, or all")
	}
	if f.Draft != "any" && f.Draft != "yes" && f.Draft != "no" {
		return f, bad("Draft must be any, yes, or no")
	}
	if f.Sort != "updated" && f.Sort != "created" && f.Sort != "comments" {
		return f, bad("Sort must be updated, created, or comments")
	}
	if f.Order != "asc" && f.Order != "desc" {
		return f, bad("Order must be asc or desc")
	}
	// Text is deliberately not an alternate qualifier language. Structured filters
	// remain authoritative and cannot be bypassed with OR/is:issue in q.
	if len(f.Q) > 256 || strings.ContainsAny(f.Q, ":()\r\n\x00") {
		return f, bad("Search accepts plain text up to 256 bytes; use the filter controls for qualifiers")
	}
	for _, key := range []string{"page", "per_page"} {
		if s := v.Get(key); s != "" {
			n, e := strconv.Atoi(s)
			if e != nil || n < 1 {
				return f, bad("Invalid " + key)
			}
			if key == "page" {
				f.Page = n
			} else {
				f.PerPage = n
			}
		}
	}
	if f.PerPage > 100 || f.Page > 1000 || (f.Page-1)*f.PerPage >= 1000 {
		return f, bad("Search pages must stay within GitHub's first 1,000 results; per_page is at most 100")
	}
	ors := 0
	for _, g := range []struct {
		key    string
		target *[]string
		re     *regexp.Regexp
	}{{"org", &f.Orgs, login}, {"repo", &f.Repos, repository}, {"author", &f.Authors, authorLogin}} {
		seen := map[string]bool{}
		for _, s := range v[g.key] {
			s = strings.TrimSpace(s)
			if !g.re.MatchString(s) || strings.HasSuffix(s, "/..") || strings.HasSuffix(s, "/.") {
				return f, bad("Invalid " + g.key)
			}
			k := strings.ToLower(s)
			if !seen[k] {
				*g.target = append(*g.target, s)
				seen[k] = true
			}
		}
		if len(*g.target) > 1 {
			ors += len(*g.target) - 1
		}
	}
	if ors > 5 {
		return f, bad("This search supports at most five OR alternatives across filters; reduce the selections")
	}
	return f, nil
}
func (f Filters) Query() string {
	parts := []string{"is:pr"}
	if f.State != "all" {
		parts = append(parts, "state:"+f.State)
	}
	if f.Draft != "any" {
		parts = append(parts, "draft:"+strconv.FormatBool(f.Draft == "yes"))
	}
	for _, g := range []struct {
		key    string
		values []string
	}{{"org", f.Orgs}, {"repo", f.Repos}, {"author", f.Authors}} {
		xs := []string{}
		for _, v := range g.values {
			xs = append(xs, g.key+":"+v)
		}
		if len(xs) == 1 {
			parts = append(parts, xs[0])
		} else if len(xs) > 1 {
			parts = append(parts, "("+strings.Join(xs, " OR ")+")")
		}
	}
	if f.Q != "" {
		parts = append(parts, strconv.Quote(f.Q))
	}
	return strings.Join(parts, " ")
}

type Service struct {
	client  *http.Client
	base    string
	resolve func(context.Context) (string, error)
	mu      sync.Mutex
	token   string
}

func New() *Service {
	return &Service{client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}, base: "https://api.github.com", resolve: credentials}
}
func credentials(ctx context.Context) (string, error) {
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if token := strings.TrimSpace(os.Getenv(key)); token != "" {
			return token, nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "gh", "auth", "token", "--hostname", "github.com").Output()
	if err != nil || strings.TrimSpace(string(b)) == "" {
		return "", &APIError{Status: 401, Code: "authentication_required", Message: "Sign in with gh auth login for github.com, or provide GH_TOKEN to Iugum"}
	}
	return strings.TrimSpace(string(b)), nil
}
func (s *Service) get(ctx context.Context, path string, v url.Values, out any) error {
	s.mu.Lock()
	if s.token == "" {
		token, err := s.resolve(ctx)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		s.token = token
	}
	token := s.token
	s.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+path+"?"+v.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "iugum-beadview")
	r, err := s.client.Do(req)
	if err != nil {
		return &APIError{Status: 502, Code: "github_unavailable", Message: "GitHub could not be reached. Try Refresh."}
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		e := &APIError{Status: 502, Code: "github_error", Message: "GitHub could not complete the request"}
		switch r.StatusCode {
		case 401:
			s.mu.Lock()
			s.token = ""
			s.mu.Unlock()
			e.Status = 401
			e.Code = "authentication_required"
			e.Message = "GitHub rejected the credential. Check gh auth status and refresh."
		case 403:
			e.Status = 403
			e.Code = "permission_denied"
			e.Message = "GitHub denied access. Check token permissions and organization authorization."
		case 404:
			e.Status = 404
			e.Code = "not_found"
			e.Message = "Pull request not found or not accessible"
		case 422:
			e.Status = 422
			e.Code = "github_query_rejected"
			e.Message = "GitHub rejected this search. Reduce filters and check repository access."
		}
		if r.StatusCode == 429 || (r.StatusCode == 403 && (r.Header.Get("X-RateLimit-Remaining") == "0" || r.Header.Get("Retry-After") != "")) {
			e.Status = 429
			e.Code = "rate_limited"
			e.Message = "GitHub rate limit reached. Wait before refreshing."
			e.RetryAfter = r.Header.Get("Retry-After")
			e.Reset = r.Header.Get("X-RateLimit-Reset")
		}
		return e
	}
	if err = json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(out); err != nil {
		return &APIError{Status: 502, Code: "invalid_github_response", Message: "GitHub returned an unreadable response"}
	}
	return nil
}

type record struct {
	Number        int    `json:"number"`
	Title         string `json:"title"`
	HTMLURL       string `json:"html_url"`
	RepositoryURL string `json:"repository_url"`
	State         string `json:"state"`
	Draft         bool   `json:"draft"`
	UpdatedAt     string `json:"updated_at"`
	Body          string `json:"body"`
	Merged        *bool  `json:"merged"`
	User          struct {
		Login string `json:"login"`
	} `json:"user"`
	PullRequest *struct {
		URL string `json:"url"`
	} `json:"pull_request"`
	Base struct {
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
}

func normalize(r record, repo string) PR {
	return PR{Number: r.Number, Repository: repo, Title: r.Title, URL: r.HTMLURL, Author: r.User.Login, State: r.State, Draft: r.Draft, UpdatedAt: r.UpdatedAt, Review: "not_loaded", Checks: "not_loaded"}
}
func (s *Service) Search(ctx context.Context, f Filters) (Result, error) {
	v := url.Values{"advanced_search": {"true"}, "q": {f.Query()}, "sort": {f.Sort}, "order": {f.Order}, "page": {strconv.Itoa(f.Page)}, "per_page": {strconv.Itoa(f.PerPage)}}
	var data struct {
		Items      []record `json:"items"`
		Total      int      `json:"total_count"`
		Incomplete bool     `json:"incomplete_results"`
	}
	err := s.get(ctx, "/search/issues", v, &data)
	if err != nil {
		return Result{}, err
	}
	result := Result{Items: []PR{}, Total: data.Total, Filters: f, Page: f.Page, PerPage: f.PerPage, Incomplete: data.Incomplete, Truncated: data.Total > 1000, FetchedAt: time.Now().UTC()}
	for _, r := range data.Items {
		if r.PullRequest == nil {
			return Result{}, &APIError{Status: 502, Code: "invalid_github_response", Message: "GitHub returned a non-PR search result"}
		}
		repo := strings.TrimPrefix(r.RepositoryURL, s.base+"/repos/")
		if !repository.MatchString(repo) {
			return Result{}, errors.New("invalid repository in GitHub response")
		}
		result.Items = append(result.Items, normalize(r, repo))
	}
	limit := min(data.Total, 1000)
	result.HasNext = f.Page*f.PerPage < limit && len(result.Items) == f.PerPage
	return result, nil
}
func (s *Service) Detail(ctx context.Context, owner, repo, number string) (PR, error) {
	if !repository.MatchString(owner+"/"+repo) || repo == ".." || repo == "." {
		return PR{}, bad("Invalid repository")
	}
	n, err := strconv.Atoi(number)
	if err != nil || n < 1 {
		return PR{}, bad("Invalid pull request number")
	}
	var r record
	if err = s.get(ctx, fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, n), nil, &r); err != nil {
		return PR{}, err
	}
	p := normalize(r, owner+"/"+repo)
	p.Body = r.Body
	p.Merged = r.Merged
	return p, nil
}
