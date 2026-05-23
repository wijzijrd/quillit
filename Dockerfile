FROM alpine:3.21
RUN mkdir -p /data
COPY bin/quillit-auth-svc /usr/local/bin/quillit-auth-svc
ENV PORT=3002
ENV DB_PATH=/data/quillit-auth.db
EXPOSE 3002
CMD ["quillit-auth-svc"]
