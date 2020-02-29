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
docker rmi -f petrjahoda/state_service:"$1"
docker build -t petrjahoda/state_service:"$1" .
docker push petrjahoda/state_service:"$1"