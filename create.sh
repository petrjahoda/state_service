#!/usr/bin/env bash
docker rmi -f petrjahoda/state_service:"$1"
docker build -t petrjahoda/state_service:"$1" .
docker push petrjahoda/state_service:"$1"