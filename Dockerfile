FROM alpine:3.21
RUN mkdir -p /data
COPY bin/quillit-svc /usr/local/bin/quillit-svc
ENV PORT=3000
ENV DB_PATH=/data/quillit.db
ENV COOKIE_SECURE=true
EXPOSE 3000
CMD ["quillit-svc"]
