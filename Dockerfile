FROM ubuntu:latest@sha256:04f510bf1f2528604dc2ff46b517dbdbb85c262d62eacc4aa4d3629783036096 AS base
RUN useradd -u 1001 kube-rbac-extractor

FROM scratch
COPY --from=base /etc/passwd /etc/passwd
COPY kube-rbac-extractor /
USER 1001
ENTRYPOINT ["/kube-rbac-extractor"]
