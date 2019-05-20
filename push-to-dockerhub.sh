#!/bin/bash
#docker login
docker build -t ma-nf .
docker tag ma-nf jhamfler/ma-nf
docker push jhamfler/ma-nf
