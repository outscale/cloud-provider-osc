#!/bin/bash
set -ex
oldbranch=`git rev-parse --abbrev-ref HEAD`
git co main
git pull --rebase
for branch in `git branch | egrep 'kubernetes-'`; do
    echo "** $branch"
    git co $branch
    git pull --rebase
    git rebase main
done
git co $oldbranch