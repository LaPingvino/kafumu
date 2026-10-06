#!/bin/sh
# Deletes old App Engine versions of the default service, keeping the newest
# KEEP (default 10) and whichever one serves traffic. App Engine caps the
# number of versions (~210); at a deploy or two an hour the cap is reached
# in days, and then deploys fail. Old code stays in git: a rollback is a
# redeploy of an old commit.
#
#   sh tools/prune-versions.sh          # shows what it would delete
#   sh tools/prune-versions.sh --yes    # deletes them
set -e
KEEP=${KEEP:-10}
GCLOUD=${GCLOUD:-$HOME/google-cloud-sdk/bin/gcloud}
PROJECT=${PROJECT:-lokumo}
old=$($GCLOUD app versions list --project "$PROJECT" --service default --sort-by=~version.createTime \
  --format='value(version.id,traffic_split)' | awk -v keep="$KEEP" 'NR > keep && $2 == 0 {print $1}')
n=$(echo "$old" | grep -c . || true)
echo "$n old versions to delete (keeping the newest $KEEP and the serving one)."
[ "$n" -eq 0 ] && exit 0
if [ "$1" != "--yes" ]; then
  echo "$old" | head -5; echo "…"
  echo "Run again with --yes to delete them."
  exit 0
fi
# In batches: one call per 20 versions.
echo "$old" | xargs -n 20 $GCLOUD app versions delete --project "$PROJECT" --service default --quiet
