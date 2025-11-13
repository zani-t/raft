#!/bin/bash

set -x
set -e

logfile=/tmp/raftlog

cd src && go test -v -race -run TestElectionFollowerComesBack | tee ${logfile}
cd ..