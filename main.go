package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/go-github/v47/github"
	"golang.org/x/oauth2"
)

var orgName = os.Getenv("BYELINEAR_ORG")
var repoName = os.Getenv("BYELINEAR_REPO")

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Minute*30)
	defer cancel()

	gchttp := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	))
	gc := github.NewClient(gchttp)

	lc := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("LINEAR_API_KEY")},
	))
	before := os.Getenv("BYELINEAR_BEFORE")
	for {
		startCursor, err := exportNextPage(ctx, lc, before, gc)
		if err != nil {
			log.Printf("before: %q: %v", before, err)
			time.Sleep(time.Minute)
			continue
		}

		if startCursor == "" {
			return
		}
		before = startCursor
		time.Sleep(time.Second * 2)
	}
}

func exportNextPage(ctx context.Context, lc *http.Client, startCursor string, gc *github.Client) (before string, _ error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute*2)
	defer cancel()

	pageSize, err := strconv.Atoi(os.Getenv("BYELINEAR_PAGE_SIZE"))
	if err != nil {
		pageSize = 1
	}
	page, before, err := queryLinearIssuesPage(ctx, lc, pageSize, startCursor)
	if err != nil {
		return "", err
	}

	for i := len(page) - 1; i >= 0; i-- {
		liss := page[i]
		iss := fromLinearIssue(liss)

		log.SetPrefix(liss.Identifier + ": ")
		url, err := exportToGithub(ctx, gc, liss.Identifier, iss)
		if err != nil {
			return "", err
		}

		log.Printf("%s: url: %s", liss.Identifier, url)
		log.Printf("%s: id: %s", liss.Identifier, liss.ID)
		before = liss.ID
	}

	return before, nil
}
