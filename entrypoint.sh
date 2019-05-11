#!/bin/bash
ips=()
ip=

echo started
echo NF: $NFDOMAIN
echo ip: $NFIP

if [ -z "$NFDOMAIN" ]
then
        NFDOMAIN=amf.default.svc.cluster.local
fi
echo NFDOMAIN: "$NFNAME"

if [ -z "$NFIP" ]
then
        ips=($(dig +short $NFDOMAIN))
        ips=("${ips[@]%%:*}")

        ip=$ips
        echo found ip: $ips
else
        ip=$NFIP
fi

echo starting Network Function: $NFDOMAIN
echo NFservices --https_addr "$NFDOMAIN"":4430"
echo MY_POD_NAME: $MY_POD_NAME
echo MY_POD_NAMESPACE: $MY_POD_NAMESPACE
echo MY_POD_IP: $MY_POD_IP
echo hostname -i: $(hostname -i)
env
cd /usr/local/bin/
echo $(pwd)
#./NFservices --https_addr "$NFIP"":4430"
prlimit --nofile=90000 ./NFservices
