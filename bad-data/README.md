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

Note: the databases below have the data record `{"ip": value, "after": "test"}`,
where the value has a type that the spec does not allow in a database. Each
database opens, and a lookup of any IPv4 address finds the record. Readers
should return an error when they decode the record, and when they skip the value
to read `after`.

- `libmaxminddb/libmaxminddb-extended-type-0.mmdb` has a value with extended
  type byte 0. The spec uses the extended type byte only for types 8 and up. A
  reader that adds 7 to the byte without a check reads the value as a map.
- `libmaxminddb/libmaxminddb-extended-type-9.mmdb` has a value with extended
  type byte 9. The spec defines no type 16.
- `libmaxminddb/libmaxminddb-container-type.mmdb` has a value of the deprecated
  data cache container type (12).
- `libmaxminddb/libmaxminddb-end-marker-type.mmdb` has a value of the deprecated
  end marker type (13).

If you find a corrupt test-sized database that crashes a MMDB reader library,
please feel free to add it here by creating a pull request.
