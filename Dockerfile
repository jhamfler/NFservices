FROM ubuntu

RUN apt-get update
RUN apt-get install -y dnsutils
COPY target/debug/producer /usr/local/bin/
COPY entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/producer
RUN chmod +x /usr/local/bin/entrypoint.sh
ENTRYPOINT ["entrypoint.sh"]
CMD []
#EXPOSE 8080
