#!/bin/sh
IMAGE=k8r.eu/justjanne/statsbot-frontend
TAGS=$(git describe --always --tags HEAD)
DEPLOYMENT=statsbot-frontend-staging
POD=statsbot-frontend-staging

kubectl set image deployment/$DEPLOYMENT $POD=$IMAGE:$TAGS