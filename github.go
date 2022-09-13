package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v47/github"
)

func exportToGithub(ctx context.Context, gc *github.Client, ident string, iss *githubIssue) (string, error) {
	issReq := &github.IssueRequest{
		Title:    &iss.title,
		Assignee: &iss.assignee,
		Body:     &iss.body,
	}
	if len(iss.labels) > 0 {
		issReq.Labels = new([]string)
	}
	for _, l := range iss.labels {
		*issReq.Labels = append(*issReq.Labels, l.name)

		log.Printf("%s: ensuring label: %s", ident, l.name)
		color := strings.TrimPrefix(l.color, "#")
		err := ensureLabel(ctx, gc, l.name, color, l.desc)
		if err != nil {
			return "", err
		}
	}
	log.Printf("%s: creating", ident)
	giss, _, err := gc.Issues.Create(ctx, orgName, repoName, issReq)
	if err != nil {
		return "", err
	}
	if iss.state == "Done" || iss.state == "Canceled" {
		issReq.State = github.String("closed")
		issReq.StateReason = github.String("completed")
		if iss.state == "Canceled" {
			issReq.StateReason = github.String("not_planned")
		}
		_, _, err = gc.Issues.Edit(ctx, orgName, repoName, *giss.Number, issReq)
		if err != nil {
			return "", err
		}
	}
	for i, c := range iss.comments {
		log.Printf("%s: creating comment %d", ident, i)
		_, _, err = gc.Issues.CreateComment(ctx, orgName, repoName, *giss.Number, &github.IssueComment{
			Body: &c,
		})
		if err != nil {
			return "", err
		}
	}
	if iss.project != nil {
		log.Printf("%s: ensuring project: %s", ident, iss.project.name)
		pID, pnum, err := ensureProject(ctx, gc.Client(), iss.project.name, iss.project.desc)
		if err != nil {
			return "", err
		}
		si, err := queryStatusField(ctx, gc.Client(), pnum)
		if err != nil {
			return "", err
		}
		itemID, err := addIssueToProject(ctx, gc.Client(), pID, *giss.NodeID)
		if err != nil {
			return "", err
		}
		err = setProjectIssueStatus(ctx, gc.Client(), pID, itemID, si, iss.state)
		if err != nil {
			return "", err
		}
	}
	return giss.GetHTMLURL(), nil
}

type githubLabel struct {
	name  string
	color string
	desc  string
}

type githubIssue struct {
	title    string
	assignee string
	body     string
	state    string
	project  *githubProject
	labels   []*githubLabel
	comments []string
}

type githubProject struct {
	name string
	desc string
}

func fromLinearIssue(liss *linearIssue) *githubIssue {
	body := fmt.Sprintf(`field | value
| - | - |
url | %s
author | @%s
date | %s
state | %s
project | %s
priority | %s
assignee | %s
labels | %s
related | %s
parent | %s
children | %s
PRs | %s
attachments | %s
`,
		liss.URL,
		emailsToGithubMap[liss.Creator.Email],
		formatTime(liss.CreatedAt),
		liss.State.Name,
		liss.Project.Name,
		liss.PriorityLabel,
		liss.assignee(),

		formatArr(liss.labelsArr()),
		formatArr(liss.relationsArr()),
		liss.Parent.Identifier,
		formatArr(liss.childrenArr()),
		formatArr(liss.prs()),

		formatArr(liss.attachmentsArr()),
	)
	if liss.Description != "" {
		body += "\n" + liss.Description
	}

	iss := &githubIssue{
		title: fmt.Sprintf("%s: %s", liss.Identifier, liss.Title),
		body:  body,
		state: liss.State.Name,
	}

	for i := len(liss.Comments.Nodes) - 1; i >= 0; i-- {
		c := liss.Comments.Nodes[i]
		iss.comments = append(iss.comments, fmt.Sprintf(`field | value
|-|-|
url | %s
author | @%s
date | %s

%s`,
			c.URL,
			emailsToGithubMap[c.User.Email],
			formatTime(c.CreatedAt),
			c.Body,
		))
	}

	if liss.Project.Name != "" {
		iss.project = &githubProject{
			name: liss.Project.Name,
			desc: liss.Project.Desc,
		}
	}
	if liss.Assignee != nil {
		iss.assignee = emailsToGithubMap[liss.Assignee.Email]
	}
	for _, linearLabel := range liss.Labels.Nodes {
		iss.labels = append(iss.labels, &githubLabel{
			name:  linearLabel.Name,
			color: linearLabel.Color,
			desc:  linearLabel.Description,
		})
	}
	return iss
}

var emailsToGithubMap = map[string]string{
	"gavin@terrastruct.com":     "gavin-ts",
	"alex@terrastruct.com":      "alixander",
	"katherine@terrastruct.com": "katwangy",
	"julio@terrastruct.com":     "ejulio-ts",
	"bernard@terrastruct.com":   "berniexie",
	"anmol@terrastruct.com":     "nhooyr",
}

type organization struct {
	id       string
	projects []*project
}

type project struct {
	id     string
	number int
	title  string
	desc   string
}

func queryOrganization(ctx context.Context, hc *http.Client) (*organization, error) {
	queryString := `query($login: String!) {
		organization(login: $login) {
			id
			projectsV2(first: 50) {
				nodes {
					id
					title
					shortDescription
					number
				}
			}
		}
	}`
	var queryResp struct {
		Data struct {
			Organization struct {
				ID         string `json:"id"`
				ProjectsV2 struct {
					Nodes []struct {
						ID     string `json:"id"`
						Title  string `json:"title"`
						Desc   string `json:"shortDescription"`
						Number int    `json:"number"`
					} `json:"nodes"`
				} `json:"projectsv2"`
			} `json:"organization"`
		} `json:"data"`
	}

	qreq := &graphqlQuery{
		Query:     queryString,
		Variables: map[string]interface{}{"login": orgName},
	}
	err := doGithubQuery(ctx, hc, qreq, &queryResp)
	if err != nil {
		return nil, err
	}

	org := &organization{
		id: queryResp.Data.Organization.ID,
	}
	for _, lp := range queryResp.Data.Organization.ProjectsV2.Nodes {
		p := &project{
			id:     lp.ID,
			title:  lp.Title,
			desc:   lp.Desc,
			number: lp.Number,
		}
		org.projects = append(org.projects, p)
	}
	return org, nil
}

type statusFieldInfo struct {
	id string

	todoID       string
	inProgressID string
	doneID       string
}

func queryStatusField(ctx context.Context, hc *http.Client, pnum int) (*statusFieldInfo, error) {
	queryString := `query($login: String!, $projectNumber: Int!) {
		organization(login: $login) {
			projectV2(number: $projectNumber) {
				field(name: "Status") {
					... on ProjectV2SingleSelectField {
						id
						options {
							id
							name
						}
					}
				}
			}
		}
	}`

	var queryResp struct {
		Data struct {
			Organization struct {
				ProjectV2 struct {
					Field struct {
						ID      string `json:"id"`
						Options []struct {
							ID   string `json:"ID"`
							Name string `json:"name"`
						} `json:"options"`
					} `json:"field"`
				} `json:"projectv2"`
			} `json:"organization"`
		} `json:"data"`
	}

	qreq := &graphqlQuery{
		Query:     queryString,
		Variables: map[string]interface{}{"login": orgName, "projectNumber": pnum},
	}
	err := doGithubQuery(ctx, hc, qreq, &queryResp)
	if err != nil {
		return nil, err
	}
	si := &statusFieldInfo{
		id: queryResp.Data.Organization.ProjectV2.Field.ID,
	}
	for _, o := range queryResp.Data.Organization.ProjectV2.Field.Options {
		switch o.Name {
		case "Todo":
			si.todoID = o.ID
		case "In Progress":
			si.inProgressID = o.ID
		case "Done":
			si.doneID = o.ID
		}
	}
	return si, nil
}

func setProjectIssueStatus(ctx context.Context, hc *http.Client, projectID, issID string, si *statusFieldInfo, linearState string) error {
	var optionID string
	switch linearState {
	case "Backlog":
		return nil
	case "Todo":
		optionID = si.todoID
	case "In Progress", "In Review":
		optionID = si.inProgressID
	case "Done", "Canceled":
		optionID = si.doneID
	default:
		return nil
	}
	queryString := `mutation($projectId: ID!, $itemId: ID!, $fieldId: ID!, $optionId: String) {
		updateProjectV2ItemFieldValue(input: {projectId: $projectId, itemId: $itemId, fieldId: $fieldId, value: { singleSelectOptionId: $optionId }}) {
			clientMutationId
		}
	}`
	qreq := &graphqlQuery{
		Query:     queryString,
		Variables: map[string]interface{}{"projectId": projectID, "itemId": issID, "fieldId": si.id, "optionId": optionID},
	}
	return doGithubQuery(ctx, hc, qreq, nil)
}

func ensureProject(ctx context.Context, hc *http.Client, name, desc string) (string, int, error) {
	org, err := queryOrganization(ctx, hc)
	if err != nil {
		return "", 0, err
	}

	var p *project
	for _, p2 := range org.projects {
		if p2.title == name {
			p = p2
		}
	}

	var pID string
	var pnum int
	if p != nil {
		pID = p.id
		pnum = p.number
		if p.desc == desc {
			return pID, pnum, nil
		}
	} else {
		pID, pnum, err = createProject(ctx, hc, org.id, name)
		if err != nil {
			return "", 0, err
		}
	}

	queryString := `mutation($projectId: ID!, $shortDescription: String) {
		updateProjectV2(input: {projectId: $projectId, shortDescription: $shortDescription}) {
			clientMutationId
		}
	}`
	qreq := &graphqlQuery{
		Query:     queryString,
		Variables: map[string]interface{}{"projectId": pID, "shortDescription": desc},
	}
	err = doGithubQuery(ctx, hc, qreq, nil)
	if err != nil {
		return "", 0, err
	}
	return pID, pnum, nil
}

func createProject(ctx context.Context, hc *http.Client, orgID string, name string) (string, int, error) {
	queryString := `mutation($title: String!, $owner: ID!) {
		createProjectV2(input: {title: $title, ownerId: $owner}) {
			projectV2 {
				id
				number
			}
		}
	}`
	var queryResp struct {
		Data struct {
			CreateProjectV2 struct {
				ProjectV2 struct {
					ID     string `json:"id"`
					Number int    `json:"number"`
				} `json:"projectV2"`
			} `json:"createProjectV2"`
		} `json:"data"`
	}

	qreq := &graphqlQuery{
		Query:     queryString,
		Variables: map[string]interface{}{"title": name, "owner": orgID},
	}
	err := doGithubQuery(ctx, hc, qreq, &queryResp)
	if err != nil {
		return "", 0, err
	}
	return queryResp.Data.CreateProjectV2.ProjectV2.ID, queryResp.Data.CreateProjectV2.ProjectV2.Number, nil
}

func ensureLabel(ctx context.Context, gc *github.Client, name, color, desc string) error {
	_, _, err := gc.Issues.CreateLabel(ctx, orgName, repoName, &github.Label{
		Name:        &name,
		Color:       &color,
		Description: &desc,
	})
	if err != nil && !isAlreadyExistsErr(err) {
		return err
	}
	return nil
}

func isAlreadyExistsErr(err error) bool {
	var ghErr *github.ErrorResponse
	return errors.As(err, &ghErr) && len(ghErr.Errors) == 1 && ghErr.Errors[0].Code == "already_exists"
}

func doGithubQuery(ctx context.Context, hc *http.Client, qreq *graphqlQuery, resp interface{}) error {
	b, err := doGraphQLQuery(ctx, "https://api.github.com/graphql", hc, qreq)
	if err != nil {
		return err
	}

	// Github is truly terrible. Rather than set their HTTP status code to a 400 class on
	// validation errors they return an error response like this with the status 200...
	var githubErrors struct {
		Errors []struct {
			Message   string `json:"message"`
			Locations []struct {
				Line   int `json:"line"`
				Column int `json:"column"`
			} `json:"locations"`
		} `json:"errors"`
	}
	err = json.Unmarshal(b, &githubErrors)
	if err == nil && len(githubErrors.Errors) > 0 {
		return fmt.Errorf("github graphql api error: %v", githubErrors)
	}
	if resp == nil {
		return nil
	}
	return json.Unmarshal(b, &resp)
}

func formatTime(t time.Time) string {
	return t.In(time.Local).Format(time.UnixDate)
}

func addIssueToProject(ctx context.Context, hc *http.Client, pID, iID string) (string, error) {
	queryString := `mutation($projectId: ID!, $contentId: ID!) {
		addProjectV2ItemById(input: {projectId: $projectId, contentId: $contentId}) {
			item {
				id
			}
		}
	}`

	var queryResp struct {
		Data struct {
			AddProjectV2ItemById struct {
				Item struct {
					ID string `json:"id"`
				} `json:"item"`
			} `json:"addProjectV2ItemById"`
		} `json:"data"`
	}

	qreq := &graphqlQuery{
		Query:     queryString,
		Variables: map[string]interface{}{"projectId": pID, "contentId": iID},
	}
	err := doGithubQuery(ctx, hc, qreq, &queryResp)
	if err != nil {
		return "", err
	}
	return queryResp.Data.AddProjectV2ItemById.Item.ID, nil
}

func formatArr(v interface{}) string {
	s := fmt.Sprintf("%v", v)
	if s[0] == '[' && s[len(s)-1] == ']' {
		return s[1 : len(s)-1]
	}
	return s
}
