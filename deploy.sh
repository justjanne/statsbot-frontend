#!/bin/sh
IMAGE=k8r.eu/justjanne/statsbot-frontend
TAGS=$(git describe --always --tags HEAD)
DEPLOYMENT=statsbot-frontend
POD=statsbot-frontend

kubectl set image deployment/$DEPLOYMENT $POD=$IMAGE:$TAGS