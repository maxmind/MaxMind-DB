These are corrupt databases that have been known to cause problems such as
segfaults or unhandled errors on one or more MaxMind DB reader implementations.
Implementations _should_ return an appropriate error or raise an exception on
these databases.

Databases are organized into subdirectories named after the reader
implementation that exposed the issue (e.g., `libmaxminddb/`).

Note: `libmaxminddb/libmaxminddb-uint64-max-epoch.mmdb` contains a valid
database structure with `build_epoch` set to `UINT64_MAX`. It may not produce a
reader error but can cause overflow in time type conversions.

Note: the databases below repeat a key in the metadata. The spec requires unique
metadata keys, so readers should reject them. They do not crash a reader, but a
reader that keeps the first value and a reader that keeps the last value see
different metadata. The rule applies to the metadata only, not to maps in the
data section.

- `libmaxminddb/libmaxminddb-repeated-metadata-key.mmdb` repeats the top-level
  key `database_type`.
- `libmaxminddb/libmaxminddb-repeated-node-count.mmdb` repeats the top-level key
  `node_count`, which sets the size of the search tree.
- `libmaxminddb/libmaxminddb-repeated-pointer-key.mmdb` repeats `database_type`
  through a pointer to the first key string.
- `libmaxminddb/libmaxminddb-repeated-description-key.mmdb` repeats the language
  key `en` in the `description` map. The top-level keys are unique.

If you find a corrupt test-sized database that crashes a MMDB reader library,
please feel free to add it here by creating a pull request.
