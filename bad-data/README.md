These are corrupt databases that have been know to cause problems such as
segfaults or unhandled errors on one or more MaxMind DB reader
implementations. Implementations _should_ return an appropriate error
or raise an exception on these databases.

Note: `libmaxminddb/libmaxminddb-uint64-max-epoch.mmdb` contains a valid
database structure with `build_epoch` set to `UINT64_MAX`. It may not produce
a reader error but can cause overflow in time type conversions.

If you find a corrupt test-sized database that crashes a MMDB reader library,
please feel free to add it here by creating a pull request.
