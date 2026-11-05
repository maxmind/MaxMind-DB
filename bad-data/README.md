These are corrupt databases that have been known to cause problems such as
segfaults or unhandled errors on one or more MaxMind DB reader implementations.
Implementations _should_ return an appropriate error or raise an exception on
these databases.

Databases are organized into subdirectories named after the reader
implementation that exposed the issue (e.g., `libmaxminddb/`).

Note: `libmaxminddb/libmaxminddb-uint64-max-epoch.mmdb` contains a valid
database structure with `build_epoch` set to `UINT64_MAX`. It may not produce a
reader error but can cause overflow in time type conversions.

Note: `libmaxminddb/libmaxminddb-repeated-metadata-key.mmdb` and
`libmaxminddb/libmaxminddb-repeated-description-key.mmdb` repeat a metadata key
with a different value. They do not crash a reader, but a reader that keeps the
first value and a reader that keeps the last value see different metadata.
Readers should reject them.

If you find a corrupt test-sized database that crashes a MMDB reader library,
please feel free to add it here by creating a pull request.
