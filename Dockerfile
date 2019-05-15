FROM jess/img:v0.5.7
USER root
ENV USER root
ENV HOME /root
RUN apk add bash rsync jq
ADD build /usr/bin/build
