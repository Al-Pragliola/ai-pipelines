package trigger

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
)

const gitHubAccept = "application/vnd.github+json"

var gitHubAPIBaseURL = "https://api.github.com"

type Issue struct {
	Number int    `json:"number"`
	Key    string `json:"key,omitempty"` // "#42" for GitHub, "PROJ-123" for Jira
	Title  string `json:"title"`
	Body   string `json:"body"`

	// PR-specific metadata (populated by FetchGitHubReviewRequests)
	PRAuthor   string `json:"prAuthor,omitempty"`
	BaseBranch string `json:"baseBranch,omitempty"`
	HeadBranch string `json:"headBranch,omitempty"`
}

// FetchGitHubIssues fetches open issues from GitHub using the Pipeline CRD trigger spec.
func FetchGitHubIssues(ctx context.Context, gh *aiv1alpha1.GitHubTriggerSpec, token string, client *CachedClient) ([]Issue, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues?assignee=%s&state=open&per_page=100",
		gitHubAPIBaseURL, gh.Owner, gh.Repo, gh.Assignee)

	if len(gh.Labels) > 0 {
		url += "&labels=" + strings.Join(gh.Labels, ",")
	}

	body, err := client.Get(ctx, url, token, gitHubAccept)
	if err != nil {
		return nil, err
	}

	var ghIssues []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(body, &ghIssues); err != nil {
		return nil, err
	}

	issues := make([]Issue, len(ghIssues))
	for i, gh := range ghIssues {
		issues[i] = Issue{
			Number: gh.Number,
			Key:    fmt.Sprintf("#%d", gh.Number),
			Title:  gh.Title,
			Body:   gh.Body,
		}
	}
	return issues, nil
}

// FetchJiraIssues fetches issues from Jira using the Pipeline CRD trigger spec.
func FetchJiraIssues(ctx context.Context, jira *aiv1alpha1.JiraTriggerSpec, token, email string) ([]Issue, error) {
	searchURL := strings.TrimRight(jira.URL, "/") + "/rest/api/3/search/jql"

	body := fmt.Sprintf(`{"jql":%q,"fields":["summary","description"],"maxResults":50}`, jira.JQL)
	req, err := http.NewRequestWithContext(ctx, "POST", searchURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if email != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(email + ":" + token))
		req.Header.Set("Authorization", "Basic "+cred)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira api returned %d", resp.StatusCode)
	}

	var result struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary     string `json:"summary"`
				Description any    `json:"description"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	issues := make([]Issue, len(result.Issues))
	for i, ji := range result.Issues {
		number := 0
		if parts := strings.SplitN(ji.Key, "-", 2); len(parts) == 2 {
			_, _ = fmt.Sscanf(parts[1], "%d", &number)
		}

		issues[i] = Issue{
			Number: number,
			Key:    ji.Key,
			Title:  ji.Fields.Summary,
			Body:   FlattenADF(ji.Fields.Description),
		}
	}
	return issues, nil
}

// FetchGitHubReviewRequests fetches open PRs where the configured reviewer has a pending review request.
func FetchGitHubReviewRequests(ctx context.Context, spec *aiv1alpha1.GitHubPRReviewTriggerSpec, token string, client *CachedClient) ([]Issue, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls?state=open", gitHubAPIBaseURL, spec.Owner, spec.Repo)

	body, err := client.Get(ctx, url, token, gitHubAccept)
	if err != nil {
		return nil, err
	}

	var pulls []struct {
		Number             int    `json:"number"`
		Title              string `json:"title"`
		Body               string `json:"body"`
		RequestedReviewers []struct {
			Login string `json:"login"`
		} `json:"requested_reviewers"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
		Head struct {
			Ref string `json:"ref"`
		} `json:"head"`
	}
	if err := json.Unmarshal(body, &pulls); err != nil {
		return nil, err
	}

	var issues []Issue
	for _, pr := range pulls {
		for _, reviewer := range pr.RequestedReviewers {
			if reviewer.Login == spec.Reviewer {
				issues = append(issues, Issue{
					Number:     pr.Number,
					Key:        fmt.Sprintf("#PR-%d", pr.Number),
					Title:      pr.Title,
					Body:       pr.Body,
					PRAuthor:   pr.User.Login,
					BaseBranch: pr.Base.Ref,
					HeadBranch: pr.Head.Ref,
				})
				break
			}
		}
	}
	return issues, nil
}

// FlattenADF extracts plain text from Jira's Atlassian Document Format.
func FlattenADF(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case map[string]any:
		if text, ok := val["text"].(string); ok {
			return text
		}
		if content, ok := val["content"].([]any); ok {
			var parts []string
			for _, c := range content {
				if t := FlattenADF(c); t != "" {
					parts = append(parts, t)
				}
			}
			return strings.Join(parts, "\n")
		}
	}
	return ""
}
