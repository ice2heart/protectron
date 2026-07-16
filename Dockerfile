# Built by goreleaser (dockers_v2). The binary is already compiled by
# goreleaser and placed in the build context under $TARGETPLATFORM/; this
# image only assembles the runtime layout.
# Run natively on the build host (not the target arch) so no emulation is
# needed just to fetch ca-certificates.
FROM --platform=$BUILDPLATFORM alpine:3 AS certs
RUN apk add --no-cache ca-certificates

FROM scratch
ARG TARGETPLATFORM

WORKDIR /app
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY $TARGETPLATFORM/protectron /app/protectron
COPY templates/ /app/templates/

USER 65534:65534
ENTRYPOINT ["/app/protectron"]
