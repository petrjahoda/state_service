#!/usr/bin/env bash
cd linux
upx state_service_linux
cd ..

docker rmi -f petrjahoda/state_service:latest
docker build -t petrjahoda/state_service:latest .
docker push petrjahoda/state_service:latest

docker rmi -f petrjahoda/state_service:2020.3.1
docker build -t petrjahoda/state_service:2020.3.1 .
docker push petrjahoda/state_service:2020.3.1
