# byelinear

`byelinear` exports Linear issues including assignees, comments, labels, linked issues/PRs and projects to GitHub issues.

While we enjoyed Linear's slick UI and superior featureset, we ultimately decided that we
wanted our issues on the same platform with the rest of our development. GitHub issues
aren't perfect but they work well enough for us and are more tightly integrated with
GitHub. So we wrote this for our internal migration from Linear issues to GitHub issues.

It will loop through Linear issues in reverse so that the most recent issue is created
last and thus shows up first in GitHub issues.

It uses the Linear GraphQL API and the GitHub V3 and V4 APIs.

It will hit the Linear GraphQL complexity limit quite quickly downloading issues. In our
case just 20 issues. byelinear will back off and retry every 5 minutes so you can just let
it run and wait until it's done. The fetch rate ends up being about 20 issues an hour.

There is also a one second wait in between every issue when fetching from Linear and
exporting to GitHub.

You can terminate byelinear and resume later. It will start right where it left off based
on the state in `./linear-corpus/state.json` (you can change this via
`$BYELINEAR_CORPUS`).

You can also contact Linear's support and request they raise your rate limit temporarily.

## Install

```sh
go install oss.terrastruct.com/byelinear@latest
byelinear --help
```

See configuration to setup the required environment variables. Then see the example below
for how to run and what the logs look like.

## Configuration

```sh
# Location of corpus for issues fetched from Linear.
# Defaults to linear-corpus in the current directory.
export BYELINEAR_CORPUS=

# Use to fetch and export only a single issue by the linear issue number. Useful for testing.
export BYELINEAR_ISSUE_NUMBER=

# org/repo into which to import issues.
# Required when running to-github.
export BYELINEAR_ORG=terrastruct
export BYELINEAR_REPO=byelinear

# Secrets required when importing/exporting with private repos/issues.
export GITHUB_TOKEN=
export LINEAR_API_KEY=
```

## Caveats

It gets everything right except for projects and state as there are limitations in
GitHub's project API. There is no way to add a new project state/column programatically so
it tries to map incoming states to GitHub default states as best as possible.

e.g. In Review from Linear becomes In Progress on GitHub. Cancelled becomes Done.

As well, GitHub's projects API does not allow for control over workflow automations like
automatically setting an issue to In Progress when a PR is opened for it. You'll have to
manually go into the projects settings and enable the workflows there.

## Example

The following example fetches issue TER-1396 from linear and then exports it to GitHub.
Empty `$BYELINEAR_ISSUE_NUMBER` to fetch all issues.

```
$ BYELINEAR_ISSUE_NUMBER=1396 LINEAR_API_KEY=lin_api_... go run . from-linear
2022/09/14 10:43:33 TER-1396: fetched
2022/09/14 10:43:34 All linear issues fetched successfully.
2022/09/14 10:43:34 Use subcommand to-github now to export them to GitHub.
```

```
$ BYELINEAR_ISSUE_NUMBER=1396 GITHUB_TOKEN=ghp_... BYELINEAR_ORG=terrastruct BYELINEAR_REPO=byelinear-test go run . to-github
2022/09/14 10:44:01 TER-1396: exporting
2022/09/14 10:44:01 TER-1396: ensuring label: dsl
2022/09/14 10:44:01 TER-1396: ensuring label: blocked
2022/09/14 10:44:02 TER-1396: ensuring label: easy
2022/09/14 10:44:02 TER-1396: ensuring label: backend
2022/09/14 10:44:02 TER-1396: creating
2022/09/14 10:44:04 TER-1396: creating comment 0
2022/09/14 10:44:04 TER-1396: creating comment 1
2022/09/14 10:44:05 TER-1396: ensuring project: D2
2022/09/14 10:44:07 TER-1396: exported: https://github.com/terrastruct/byelinear-test/issues/1
```

### Before

![linear](./TER-1396-linear.png)

### After

![github](./TER-1396-github.png)

## Related

- [https://github.com/jccr/linear-to-gitlab](https://github.com/jccr/linear-to-gitlab)
