# The ai-flow CLI on a distroless base, built by goreleaser's dockers_v2
# (.goreleaser.yaml), which places each platform's binary under
# $TARGETPLATFORM/ in the build context. The binary is static
# (CGO_ENABLED=0), so the base carries nothing but CA certificates,
# tzdata and a nonroot user. No check tools are bundled: copy the binary
# into an image that already pins yours:
#   COPY --from=ghcr.io/pmi-ai-stack/ai-flow-cli:vX.Y.Z /usr/local/bin/ai-flow /usr/local/bin/ai-flow
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/ai-flow /usr/local/bin/ai-flow
ENTRYPOINT ["/usr/local/bin/ai-flow"]
