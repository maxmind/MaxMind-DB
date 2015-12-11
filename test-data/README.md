The write-test-dbs script will create a small set of test databases with a
variety of data and record sizes (24, 28, & 32 bit).

These test databases are useful for testing code that reads MaxMind DB files.

There is also a `maps-with-pointers.raw` file. This contains the raw output of
the MaxMind::DB::Writer::Serializer module, when given a series of maps which
share some keys and values. It is used to test that decoder code can handle
pointers to map keys and values, as well as to the whole map.

Once you have ran the perl script to generate the test databases you will need
to check what IP addresses have been loaded into the test databases which have
been generated.
You can refer to the
[souce-data directory](https://github.com/maxmind/MaxMind-DB/tree/master/source-data)
in this repository which contains the
JSON files which have been used to generate the database files.
