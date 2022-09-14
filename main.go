package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v47/github"
	"golang.org/x/oauth2"
)

var byelinearIssueNumber = os.Getenv("BYELINEAR_ISSUE_NUMBER")
var byelinearCorpus = os.Getenv("BYELINEAR_CORPUS")

var orgName = os.Getenv("BYELINEAR_ORG")
var repoName = os.Getenv("BYELINEAR_REPO")

var githubToken = os.Getenv("GITHUB_TOKEN")
var linearAPIKey = os.Getenv("LINEAR_API_KEY")

type state struct {
	Issues []*issueState `json:"issues"`
}

type issueState struct {
	ID               string `json:"id"`
	Identifier       string `json:"identifier"`
	ExportedToGithub bool   `json:"exported_to_github"`
}

func main() {
	if byelinearCorpus == "" {
		byelinearCorpus = "linear-corpus"
	}

	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

func run() (err error) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Hour*24)
	defer cancel()

	s, err := readState()
	if err != nil {
		return err
	}
	defer func() {
		err2 := writeState(s)
		if err2 != nil {
			err2 = fmt.Errorf("failed to write state: %v", err2)
			if err != nil {
				log.Print(err2)
			} else {
				err = err2
			}
		}
	}()

	sigs := make(chan os.Signal)
	signal.Notify(sigs, os.Interrupt)

	done := make(chan error, 1)
	go func() {
		defer close(done)

		if len(os.Args) < 2 {
			usage()
		}
		switch os.Args[1] {
		case "from-linear":
			done <- s.fromLinear(ctx)
		case "to-github":
			done <- s.toGithub(ctx)
		default:
			usage()
		}
	}()

	select {
	case err = <-done:
	case <-sigs:
		cancel()
		err = <-done
	}
	return err
}

func usage() {
	fmt.Printf(`usage:
	%s [ from-linear | to-github ]

Use from-linear to export issues from linear and to-github to export issues to github.
See docs and environment variable configuration at https://oss.terrastruct.com/byelinear
`, os.Args[0])
	os.Exit(1)
}

func readState() (*state, error) {
	sb, err := os.ReadFile(filepath.Join(byelinearCorpus, "state.json"))
	if os.IsNotExist(err) {
		return &state{}, nil
	}
	if err != nil {
		return nil, err
	}

	var s *state
	err = json.Unmarshal(sb, &s)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func writeState(s *state) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(byelinearCorpus, "state.json"), b, 0644)
}

func fetchLinearIssue(ctx context.Context, lc *http.Client, previousID string) (*linearIssue, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute*2)
	defer cancel()

	li, err := queryLinearIssue(ctx, lc, previousID)
	if err != nil {
		return nil, err
	}
	if li == nil {
		return nil, nil
	}

	b, err := json.Marshal(li)
	if err != nil {
		return nil, err
	}

	dest := filepath.Join(byelinearCorpus, li.Identifier+".json")
	err = os.WriteFile(dest, b, 0644)
	if err != nil {
		return nil, err
	}

	log.Printf("%s: fetched", li.Identifier)
	return li, nil
}

func (s *state) fromLinear(ctx context.Context) error {
	err := os.MkdirAll(byelinearCorpus, 0755)
	if err != nil {
		return err
	}

	lc := http.DefaultClient
	if linearAPIKey != "" {
		lc = oauth2.NewClient(ctx, oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: linearAPIKey},
		))
	}

	previousID := ""
	if len(s.Issues) > 0 {
		previousID = s.Issues[len(s.Issues)-1].ID
	}
	for {
		liss, err := fetchLinearIssue(ctx, lc, previousID)
		if err != nil {
			log.Print(err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Minute * 5):
				continue
			}
		}

		if liss == nil {
			log.Print("All linear issues fetched successfully.")
			log.Print("Use subcommand to-github now to export them to GitHub.")
			return nil
		}
		s.Issues = append(s.Issues, &issueState{
			ID:               liss.ID,
			Identifier:       liss.Identifier,
			ExportedToGithub: false,
		})
		previousID = liss.ID
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
			continue
		}
	}
}

func (s *state) toGithub(ctx context.Context) error {
	if orgName == "" {
		log.Fatalf("$BYELINEAR_ORG is required")
	}
	if repoName == "" {
		log.Fatalf("$BYELINEAR_REPO is required")
	}

	gchttp := http.DefaultClient
	if githubToken != "" {
		gchttp = oauth2.NewClient(ctx, oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: githubToken},
		))
	}
	gc := github.NewClient(gchttp)

	for _, iss := range s.Issues {
		if byelinearIssueNumber != "" && !strings.HasSuffix(iss.Identifier, "-"+byelinearIssueNumber) {
			continue
		}
		liss, err := iss.linear()
		if err != nil {
			return err
		}
		if liss.Creator == nil {
			log.Printf("%s: skipped tutorial issue", iss.Identifier)
			continue
		}
		if iss.ExportedToGithub {
			log.Printf("%s: skipped already exported issue", iss.Identifier)
			continue
		}

		log.Printf("%s: exporting", iss.Identifier)

		for {
			url, err := exportToGithub(ctx, gc, iss.Identifier, fromLinearIssue(liss))
			if err != nil {
				log.Print(err)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Minute * 5):
					continue
				}
			}

			iss.ExportedToGithub = true
			log.Printf("%s: exported: %s", iss.Identifier, url)
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
			continue
		}
	}
	return nil
}

func (is *issueState) linear() (*linearIssue, error) {
	file := filepath.Join(byelinearCorpus, is.Identifier+".json")
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	var liss *linearIssue
	err = json.Unmarshal(b, &liss)
	if err != nil {
		return nil, err
	}
	return liss, nil
}
