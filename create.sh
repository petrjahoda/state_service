#!/usr/bin/env bash
cd linux
upx state_service_linux
cd ..
cd mac
upx state_service_mac
cd ..
cd windows
upx state_service_windows.exe
cd ..

docker rmi -f petrjahoda/state_service:latest
docker build -t petrjahoda/state_service:latest .
docker push petrjahoda/state_service:latest

docker rmi -f petrjahoda/state_service:2020.1.3
docker build -t petrjahoda/state_service:2020.1.3 .
docker push petrjahoda/state_service:2020.1.3
