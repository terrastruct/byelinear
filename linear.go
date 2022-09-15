package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

func doLinearQuery(ctx context.Context, hc *http.Client, qreq *graphqlQuery, resp interface{}) error {
	b, httpResp, err := doGraphQLQuery(ctx, "https://api.linear.app/graphql", hc, qreq)
	if os.Getenv("DEBUG") != "" {
		if httpResp != nil && httpResp.Header.Get("X-Complexity") != "" {
			log.Printf("linear query with %s complexity", httpResp.Header.Get("X-Complexity"))
		}
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &resp)
}

func queryLinearIssues(ctx context.Context, hc *http.Client, before string) ([]*linearIssue, error) {
	queryString := `query($before: String, $number: Float) {
		issues(last: 50, before: $before, filter: {number: {eq: $number}}, includeArchived: true) {
			nodes {
				id
				url
				identifier
				title
				description
				creator {
					name
					email
				}
				assignee {
					name
					email
				}
				priorityLabel
				state {
					name
				}
				project {
					name
					description
				}
				createdAt
				labels(last: 10) {
					nodes {
						name
						color
						description
					}
				}
				comments(last: 10) {
					nodes {
						url
						user {
							name
							email
						}
						createdAt
						body
					}
				}
				integrationResources(last: 10) {
					nodes {
						pullRequest {
							number
							repoName
							repoLogin
						}
					}
				}
				attachments(last: 10) {
					nodes {
						url
					}
				}
				relations(last: 10) {
					nodes {
						relatedIssue {
							identifier
						}
					}
				}
				parent {
					identifier
				}
				children(last: 10) {
					nodes {
						identifier
					}
				}
			}
		}
	}`
	var queryResp struct {
		Data struct {
			Issues struct {
				Nodes []*linearIssue `json:"nodes"`
			} `json:"issues"`
		} `json:"data"`
	}

	qreq := &graphqlQuery{
		Query:     queryString,
		Variables: map[string]interface{}{},
	}
	if before != "" {
		qreq.Variables["before"] = before
	}
	number, err := strconv.Atoi(byelinearIssueNumber)
	if err == nil {
		qreq.Variables["number"] = number
	}
	err = doLinearQuery(ctx, hc, qreq, &queryResp)
	if err != nil {
		return nil, err
	}
	return queryResp.Data.Issues.Nodes, nil
}

type linearUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type linearIssue struct {
	ID            string      `json:"id"`
	URL           string      `json:"url"`
	Identifier    string      `json:"identifier"`
	Title         string      `json:"title"`
	Description   string      `json:"description"`
	Creator       *linearUser `json:"creator"`
	Assignee      *linearUser `json:"assignee"`
	PriorityLabel string      `json:"priorityLabel"`
	State         struct {
		Name string `json:"name"`
	} `json:"state"`
	Project struct {
		Name string `json:"name"`
		Desc string `json:"description"`
	} `json:"project"`
	CreatedAt time.Time `json:"createdAt"`
	Labels    struct {
		Nodes []struct {
			Name        string `json:"name"`
			Color       string `json:"color"`
			Description string `json:"description"`
		} `json:"nodes"`
	} `json:"labels"`
	Comments struct {
		Nodes []struct {
			URL       string      `json:"url"`
			User      *linearUser `json:"user"`
			CreatedAt time.Time   `json:"createdAt"`
			Body      string      `json:"body"`
		} `json:"nodes"`
	} `json:"comments"`
	Relations struct {
		Nodes []struct {
			RelatedIssue struct {
				Identifier string `json:"identifier"`
			} `json:"relatedIssue"`
		} `json:"nodes"`
	} `json:"relations"`
	IntegrationResources struct {
		Nodes []struct {
			PullRequest *struct {
				Number    int    `json:"number"`
				RepoLogin string `json:"repoLogin"`
				RepoName  string `json:"repoName"`
			} `json:"pullRequest"`
		} `json:"nodes"`
	} `json:"integrationResources"`
	Parent struct {
		Identifier string `json:"identifier"`
	} `json:"parent"`
	Children struct {
		Nodes []struct {
			Identifier string `json:"identifier"`
		} `json:"nodes"`
	} `json:"children"`
	Attachments struct {
		Nodes []struct {
			URL string `json:"url"`
		} `json:"nodes"`
	} `json:"attachments"`
}

func (li *linearIssue) labelsArr() []string {
	var a []string
	for _, l := range li.Labels.Nodes {
		a = append(a, l.Name)
	}
	return a
}

func (li *linearIssue) relationsArr() []string {
	var a []string
	for _, rel := range li.Relations.Nodes {
		a = append(a, rel.RelatedIssue.Identifier)
	}
	return a
}

func (li *linearIssue) childrenArr() []string {
	var a []string
	for _, ch := range li.Children.Nodes {
		a = append(a, ch.Identifier)
	}
	return a
}

func (li *linearIssue) assignee() string {
	if li.Assignee == nil {
		return ""
	}
	return "@" + emailsToGithubMap[li.Assignee.Email]
}

func (li *linearIssue) prs() []string {
	var prs []string
	for _, ir := range li.IntegrationResources.Nodes {
		if ir.PullRequest != nil {
			if ir.PullRequest.RepoLogin == orgName && ir.PullRequest.RepoName == repoName {
				prs = append(prs, fmt.Sprintf("#%d", ir.PullRequest.Number))
			} else {
				prs = append(prs, fmt.Sprintf("%s/%s#%d", ir.PullRequest.RepoLogin, ir.PullRequest.RepoName, ir.PullRequest.Number))
			}
		}
	}
	return prs
}

func (li *linearIssue) attachmentsArr() []string {
	var a []string
	for _, att := range li.Attachments.Nodes {
		a = append(a, att.URL)
	}
	return a
}
