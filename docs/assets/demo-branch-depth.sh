#!/bin/sh
# Count slash-separated components of the real PR head branch, locally.
# An example of computed context, not an external workflow/ticket status.
branch=$1
depth=1
while [ "${branch#*/}" != "$branch" ]; do
  branch=${branch#*/}
  depth=$((depth + 1))
done
printf 'depth:%s\n' "$depth"
