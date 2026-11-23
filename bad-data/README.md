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

Note: the databases below test where the data section ends. The data section
ends where the last metadata marker starts, so a reader must not read the marker
or the metadata as data. Each database has a search tree of one node with 24-bit
records.

These databases are valid. A reader should open them and decode their data:

- `libmaxminddb/libmaxminddb-data-section-empty.mmdb` has no data section bytes.
  Both records are empty.
- `libmaxminddb/libmaxminddb-data-section-1-byte.mmdb` has an empty map.
- `libmaxminddb/libmaxminddb-data-section-2-bytes.mmdb` has a 1-byte string.
- `libmaxminddb/libmaxminddb-data-ends-at-metadata-marker.mmdb` has a string
  that ends where the marker starts.
- `libmaxminddb/libmaxminddb-data-contains-metadata-marker.mmdb` has a string
  whose content is a metadata marker. Only the last marker in the file starts
  the metadata.

These databases open, but a lookup of `1.2.3.4` should fail when the reader
decodes the data:

- `libmaxminddb/libmaxminddb-data-section-truncated-size.mmdb` has a 2-byte data
  section. Its value has a size that needs three more size bytes.
- `libmaxminddb/libmaxminddb-data-extends-into-metadata-marker.mmdb` has a
  string that claims one byte more than the data section holds.
- `libmaxminddb/libmaxminddb-pointer-into-metadata.mmdb` has a pointer to the
  first metadata byte.
- `libmaxminddb/libmaxminddb-boolean-size-2.mmdb` has a boolean with a size
  of 2. The size field of a boolean holds its value, so only 0 and 1 are valid.

In `libmaxminddb/libmaxminddb-record-points-at-metadata-marker.mmdb`, the right
record points at the metadata marker. A lookup of `128.0.0.1` should fail.

Readers should not open these databases:

- `libmaxminddb/libmaxminddb-no-data-separator.mmdb` has no separator between
  the search tree and the metadata marker.
- `libmaxminddb/libmaxminddb-short-data-separator.mmdb` has a separator of 8
  bytes, not 16.
- `libmaxminddb/libmaxminddb-no-search-tree.mmdb` has only the metadata marker
  and valid metadata.
- `libmaxminddb/libmaxminddb-metadata-truncated-size.mmdb` has only the metadata
  marker and a 2-byte metadata section. Its value has a size that needs three
  more size bytes.

Note: `libmaxminddb/libmaxminddb-search-tree-deeper-than-address.mmdb` is an
IPv4 database with a search tree of 34 nodes. Both records of each node point to
the next node, so a lookup uses all 32 bits of the address and still ends on a
search node. Each lookup should fail with a corrupt search tree error.

If you find a corrupt test-sized database that crashes a MMDB reader library,
please feel free to add it here by creating a pull request.
