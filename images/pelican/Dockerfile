# Test-runner image for the pelican test suite: the statically-linked `pelican`
# client binary copied out of the (distroless) upstream client image, on top of
# alpine so that the pod has sh, sleep, etc. PELICAN_CLIENT_IMAGE selects the
# upstream client image (org and tag) to take the binary from.
ARG PELICAN_CLIENT_IMAGE="hub.osg-htc.org/pelican_platform/client:latest"
FROM ${PELICAN_CLIENT_IMAGE} AS pelican-client

FROM alpine:3
COPY --from=pelican-client /usr/bin/pelican /usr/local/bin/pelican

# Regular user account so that files written by the test suite aren't owned by root.
RUN adduser -D -u 1001 pelican
USER pelican
CMD ["sleep", "infinity"]
