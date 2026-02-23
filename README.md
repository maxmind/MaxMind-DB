MaxMind DB is a binary file format that stores data indexed by IP address
subnets (IPv4 or IPv6).

This repository contains the spec for that format as well as test databases.

# Generating Test Data

The `write-test-data` command generates the MMDB test files under `test-data/`
and `bad-data/`.

When run from anywhere inside this repository, it auto-detects the repo root
and uses default paths:

```bash
go run ./cmd/write-test-data
```

You can override any path with flags:

```bash
go run ./cmd/write-test-data \
  -source ./source-data \
  -target /tmp/test-out \
  -bad-data /tmp/bad-out
```

# Copyright and License

This software is Copyright (c) 2013 - 2026 by MaxMind, Inc.

This is free software, licensed under the [Apache License, Version
2.0](LICENSE-APACHE) or the [MIT License](LICENSE-MIT), at your option.
