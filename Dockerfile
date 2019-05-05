FROM ubuntu

RUN apt-get update
RUN apt-get install -y dnsutils
COPY NFservices /usr/local/bin/
COPY entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/NFservices
RUN chmod +x /usr/local/bin/entrypoint.sh
ENTRYPOINT ["entrypoint.sh"]
CMD []
#EXPOSE 8080
