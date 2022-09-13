package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

func doLinearQuery(ctx context.Context, hc *http.Client, qreq *graphqlQuery, resp interface{}) error {
	b, err := doGraphQLQuery(ctx, "https://api.linear.app/graphql", hc, qreq)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &resp)
}

func queryLinearIssuesPage(ctx context.Context, hc *http.Client, pageSize int, before string) ([]*linearIssue, string, error) {
	queryString := `query($last: Int, $before: String, $number: Float) {
		issues(last: $last, before: $before, filter: {number: {eq: $number}}, includeArchived: true) {
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
				labels {
					nodes {
						name
						color
						description
					}
				}
				comments {
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
				integrationResources {
					nodes {
						pullRequest {
							number
							repoName
							repoLogin
						}
					}
				}
				attachments {
					nodes {
						url
					}
				}
			}
			pageInfo {
				startCursor
			}
		}
	}`
	var queryResp struct {
		Data struct {
			Issues struct {
				Nodes    []*linearIssue `json:"nodes"`
				PageInfo struct {
					StartCursor string `json:"startCursor"`
				} `json:"pageInfo"`
			} `json:"issues"`
		} `json:"data"`
	}

	qreq := &graphqlQuery{
		Query:     queryString,
		Variables: map[string]interface{}{"last": pageSize},
	}
	if before != "" {
		qreq.Variables["before"] = before
	}
	// To test a specific linear issue do:
	// e.g. BYELINEAR_ISSUE_NUMBER=1396
	number, err := strconv.Atoi(os.Getenv("BYELINEAR_ISSUE_NUMBER"))
	if err == nil {
		qreq.Variables["number"] = number
	}
	err = doLinearQuery(ctx, hc, qreq, &queryResp)
	if err != nil {
		return nil, "", err
	}

	before = queryResp.Data.Issues.PageInfo.StartCursor
	// hasNextPage only reports forward, not back. So we instead just check if the query
	// found anything.
	if len(queryResp.Data.Issues.Nodes) == 0 {
		before = ""
	}

	for _, li := range queryResp.Data.Issues.Nodes {
		err = queryLinearRelations(ctx, hc, li)
		if err != nil {
			return nil, "", err
		}
	}

	return queryResp.Data.Issues.Nodes, before, nil
}

func queryLinearRelations(ctx context.Context, hc *http.Client, li *linearIssue) error {
	queryString := `query($id: ID){
		issues(filter: {id: {eq: $id}}) {
			nodes {
				relations {
					nodes {
						relatedIssue {
							identifier
						}
					}
				}
				parent {
					identifier
				}
				children {
					nodes {
						identifier
					}
				}
			}
		}
	}
	`
	var queryResp struct {
		Data struct {
			Issues struct {
				Nodes []*linearIssue `json:"nodes"`
			} `json:"issues"`
		} `json:"data"`
	}

	qreq := &graphqlQuery{
		Query:     queryString,
		Variables: map[string]interface{}{"id": li.ID},
	}
	err := doLinearQuery(ctx, hc, qreq, &queryResp)
	if err != nil {
		return err
	}

	if len(queryResp.Data.Issues.Nodes) > 0 {
		li.Relations = queryResp.Data.Issues.Nodes[0].Relations
		li.Parent = queryResp.Data.Issues.Nodes[0].Parent
		li.Children = queryResp.Data.Issues.Nodes[0].Children
	}
	return nil
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
	for i := len(li.Labels.Nodes) - 1; i >= 0; i-- {
		l := li.Labels.Nodes[i]
		a = append(a, l.Name)
	}
	return a
}

func (li *linearIssue) relationsArr() []string {
	var a []string
	for i := len(li.Relations.Nodes) - 1; i >= 0; i-- {
		rel := li.Relations.Nodes[i]
		a = append(a, rel.RelatedIssue.Identifier)
	}
	return a
}

func (li *linearIssue) childrenArr() []string {
	var a []string
	for i := len(li.Children.Nodes) - 1; i >= 0; i-- {
		a = append(a, li.Children.Nodes[i].Identifier)
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
	for i := len(li.IntegrationResources.Nodes) - 1; i >= 0; i-- {
		ir := li.IntegrationResources.Nodes[i]
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
	for i := len(li.Attachments.Nodes) - 1; i >= 0; i-- {
		att := li.Attachments.Nodes[i]
		a = append(a, att.URL)
	}
	return a
}
