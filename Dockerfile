FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY metal-seed-mutator /
ENTRYPOINT ["/metal-seed-mutator"]
