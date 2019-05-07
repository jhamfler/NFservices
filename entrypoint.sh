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
NFservices --https_addr "$NFIP"":4430"
