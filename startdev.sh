#!/bin/bash

go build && ./test2 --https_addr "127.0.0.1:4430"
#go build && ./test2
#echo go to website
#echo "https://127.0.0.1:4430/reqinfo"
#./test2
# prod
# ./test3 --prod --https_addr=server.default.svc.cluster.local
