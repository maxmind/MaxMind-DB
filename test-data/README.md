## How to generate test data

Use the
[write-test-data](https://github.com/maxmind/MaxMind-DB/tree/main/cmd/write-test-data)
go tool to create a small set of test databases with a variety of data and
record sizes.

These test databases are useful for testing code that reads MaxMind DB files.

There are several ways to figure out what IP addresses are actually in the test
databases. You can take a look at the
[source-data directory](https://github.com/maxmind/MaxMind-DB/tree/main/source-data)
in this repository. This directory contains JSON files which are used to
generate many (but not all) of the database files.

You can also use the
[mmdb-dump-database script](https://github.com/maxmind/MaxMind-DB-Reader-perl/blob/main/eg/mmdb-dump-database)
in the
[MaxMind-DB-Reader-perl repository](https://github.com/maxmind/MaxMind-DB-Reader-perl).

## Static test data

Some of the test files are remnants of the
[old perl test data writer](https://github.com/maxmind/MaxMind-DB/blob/f0a85c671c5b6e9c5e514bd66162724ee1dedea3/test-data/write-test-data.pl)
and cannot be generated with the go tool. These databases are intentionally
broken, and exploited functionality simply not available in the go mmdbwriter:

- MaxMind-DB-test-broken-pointers-24.mmdb
- MaxMind-DB-test-broken-search-tree-24.mmdb
- MaxMind-DB-test-pointer-decoder.mmdb
- GeoIP2-City-Test-Broken-Double-Format.mmdb
- GeoIP2-City-Test-Invalid-Node-Count.mmdb
- maps-with-pointers.raw

## Denial of service test data

Some files in this directory are hostile by design. Each one is structurally
well-formed, but a reader's resource policy may still refuse to open or decode
it. A reader without resource controls can use far more CPU or memory than the
file size suggests. Use these files to test the guidance in the
[Reader Resource Limits](../MaxMind-DB-spec.md#reader-resource-limits) section
of the specification.

These files do not define an exact boundary for the recommended nesting depth.
Use a reader-level test to verify the exact boundary that implementation uses.

Do not decode these files without time and memory limits on the process. The
worst-case file is about 192 KiB and describes about 4 GiB of repeated data. A
test harness that walks this whole directory and decodes every data entry will
hang or run out of memory.

| File                                                      | Behavior under test                                                                                                                      |
| --------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| MaxMind-DB-test-pointer-decoder-dos.mmdb                  | A depth-40 pointer fan-out. An unprotected decoder performs 2\*\*40 leaf decodes from 451 bytes.                                         |
| MaxMind-DB-test-pointer-decoder-dos-ipv6.mmdb             | The same fan-out in a conventional IPv6 database that maps the whole address space to the data entry.                                    |
| MaxMind-DB-test-payload-amplification-dos.mmdb            | 8,192 pointers to one 65,535-byte value of type `bytes`. A reader that copies each target materializes about 512 MiB.                    |
| MaxMind-DB-test-payload-amplification-dos-string.mmdb     | The same shape with a UTF-8 string value, the type most bindings copy into a native string.                                              |
| MaxMind-DB-test-payload-amplification-dos-worst-case.mmdb | 65,535 pointers to that value. The data entry decodes to 65,536 values, meets the recommended value limit, and materializes about 4 GiB. |

The next table lists example boundary fixtures. Its results assume the flat
value accounting rule and the 2 MiB payload limit described below.

| File                                                   | Expected result                                                                 |
| ------------------------------------------------------ | ------------------------------------------------------------------------------- |
| MaxMind-DB-test-decoder-value-limit.mmdb               | Accept. 65,536 decoded values, exactly the limit.                               |
| MaxMind-DB-test-decoder-value-limit-over.mmdb          | Reject. 65,537 decoded values.                                                  |
| MaxMind-DB-test-decoder-value-limit-pointer-heavy.mmdb | Accept. 65,535 values reached through a depth-15 fan-out.                       |
| MaxMind-DB-test-decoder-payload-limit.mmdb             | Accept. 2,097,152 payload bytes, exactly 2 MiB.                                 |
| MaxMind-DB-test-decoder-payload-limit-over.mmdb        | Reject. 2,097,153 payload bytes.                                                |
| MaxMind-DB-test-metadata-payload-limit.mmdb            | Reject at open. The metadata alone materializes 2,228,190 bytes.                |
| MaxMind-DB-test-decode-path-shared-budget.mmdb         | Reject a path lookup. Navigation plus the selected value costs 2,097,153 bytes. |

### The boundary values are recommendations

The spec recommends a limit of 65,536 decoded values. It does not require a
single payload limit, because the right method and limit depend on the reader's
language and API.

The payload files use 2 MiB (2\*\*21 bytes), matching the default in the
companion [libmaxminddb](https://github.com/maxmind/libmaxminddb/pull/479) pull
request and the limits in the companion
[Go](https://github.com/oschwald/maxminddb-golang/pull/233),
[Python](https://github.com/maxmind/MaxMind-DB-Reader-python/pull/439),
[Ruby](https://github.com/maxmind/MaxMind-DB-Reader-ruby/pull/235),
[PHP](https://github.com/maxmind/MaxMind-DB-Reader-php/pull/281),
[Java](https://github.com/maxmind/MaxMind-DB-Reader-java/pull/442), and
[.NET](https://github.com/maxmind/MaxMind-DB-Reader-dotnet/pull/355) reader pull
requests. This shared test boundary is not a file format requirement. A reader
that picks a different payload limit or uses an equivalent strategy is still
compliant. Treat these files as examples of the attack shape and adjust the
expected boundary to the reader's policy.

### The payload files assume per-occurrence accounting

The payload boundary files point 32 of their 33 elements at one shared value.
They separate accept from reject only for a reader that charges every decoded
occurrence.

A reader that safely memoizes pointer targets can materialize the shared value
one time, about 64 KiB, and accept both files. That behavior is compliant. The
result can still contain many logical references to the shared value. A binding,
serializer, or conversion to owning values that copies each occurrence should
apply its own bound. The amplification files above can test that downstream
boundary.

The pointer fan-out files test a reader without safe reuse or another work
bound. The worst-case payload file tests a reader whose only defense is the
decoded-value count.

## Usage

```
Usage of ./write-test-data:
  -source string
        Source data directory
  -target string
        Destination directory for the generated mmdb files
```

Example: `./write-test-data --source ../../source-data --target ../../test-data`
