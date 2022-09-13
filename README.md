# byelinear

byelinear exports Linear issues including assignees, comments, labels, linked issues/PRs and projects to GitHub issues.

We wrote this for our migration back from Linear issues to GitHub issues.

![cpu usage](./cpu.png)

It will hit the Linear GraphQL complexity limit quite quickly. In our case just 100
issues. byelinear will back off and retry every minute so you can just let it run and
wait until it's done.

Or you can terminate byelinear and then later set `$BYELINEAR_BEFORE` to the ID of the
last successfully exported issue to resume right where you left off. You can find the
ID in the logs.

You can also contact Linear's support support and request they raise your rate limit
temporarily.

It will loop through Linear issues in reverse so that the most recent issue is created
last and thus shows up first in GitHub issues.

It uses the Linear GraphQL API and the GitHub V3 and V4 APIs.

## Install

```sh
go install oss.terrastruct.com/byelinear@latest
```

See configuration to setup the required environment variables and then just run
`byelinear` and away you go. See the example below too for what the logs look like
and the expected output.

## Configuration

```sh
# Use to resume export with ID of last successfully exported issue. See logs for ID.
# It's BEFORE because we paginate in reverse as we want most recent issues created last.
# Optional.
export BYELINEAR_BEFORE=

# Size of pages to fetch from linear.
# Optional, default is 10.
export BYELINEAR_PAGE_SIZE=10

# org/repo into which to import issues.
# Required.
export BYELINEAR_ORG=terrastruct
export BYELINEAR_REPO=byelinear

# Required secrets
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

```
$ byelinear
2022/09/13 11:54:10 TER-12: ensuring label: Improvement
2022/09/13 11:54:10 TER-12: creating
2022/09/13 11:54:11 TER-12: url: https://github.com/terrastruct/byelinear-test/issues/4
2022/09/13 11:54:11 TER-12: id: 6d22a171-3299-4e31-9f74-6d61d476538e
```

### Before

![linear](./TER-1396-linear.png)

### After

![github](./TER-1396-github.png)
