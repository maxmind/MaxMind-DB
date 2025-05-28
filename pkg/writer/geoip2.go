package writer

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"go4.org/netipx"
)

// WriteGeoIP2TestDB writes GeoIP2 test mmdb files.
func (w *Writer) WriteGeoIP2TestDB() error {
	// This is a map from database type to input file name.
	dbTypes := map[string]string{
		"GeoIP-Anonymous-Plus":               "GeoIP-Anonymous-Plus-Test.json",
		"GeoIP2-Anonymous-IP":                "GeoIP2-Anonymous-IP-Test.json",
		"GeoIP2-City":                        "GeoIP2-City-Test.json",
		"GeoIP2-City-Shield":                 "GeoIP2-City-Test.json",
		"GeoIP2-Connection-Type":             "GeoIP2-Connection-Type-Test.json",
		"GeoIP2-Country":                     "GeoIP2-Country-Test.json",
		"GeoIP2-Country-Shield":              "GeoIP2-Country-Test.json",
		"GeoIP2-DensityIncome":               "GeoIP2-DensityIncome-Test.json",
		"GeoIP2-Domain":                      "GeoIP2-Domain-Test.json",
		"GeoIP2-Enterprise":                  "GeoIP2-Enterprise-Test.json",
		"GeoIP2-Enterprise-Shield":           "GeoIP2-Enterprise-Test.json",
		"GeoIP2-IP-Risk":                     "GeoIP2-IP-Risk-Test.json",
		"GeoIP2-ISP":                         "GeoIP2-ISP-Test.json",
		"GeoIP2-Precision-Enterprise":        "GeoIP2-Precision-Enterprise-Test.json",
		"GeoIP2-Precision-Enterprise-Shield": "GeoIP2-Precision-Enterprise-Test.json",
		"GeoIP2-Static-IP-Score":             "GeoIP2-Static-IP-Score-Test.json",
		"GeoIP2-User-Count":                  "GeoIP2-User-Count-Test.json",
		"GeoLite2-ASN":                       "GeoLite2-ASN-Test.json",
		"GeoLite2-City":                      "GeoLite2-City-Test.json",
		"GeoLite2-Country":                   "GeoLite2-Country-Test.json",
	}

	for dbType, jsonFileName := range dbTypes {
		languages := []string{"en"}
		description := map[string]string{
			"en": strings.ReplaceAll(dbType, "-", " ") +
				" Test Database (fake GeoIP2 data, for example purposes only)",
		}

		if dbType == "GeoIP2-City" {
			languages = append(languages, "zh")
			description["zh"] = "小型数据库"
		}

		dbWriter, err := mmdbwriter.New(
			mmdbwriter.Options{
				DatabaseType:        dbType,
				Description:         description,
				DisableIPv4Aliasing: false,
				IPVersion:           6,
				Languages:           languages,
				RecordSize:          28,
			},
		)
		if err != nil {
			return fmt.Errorf("creating mmdbwriter: %w", err)
		}

		if dbType == "GeoIP2-Anonymous-IP" || dbType == "GeoIP-Anonymous-Plus" {
			if err := populateAllNetworks(dbWriter); err != nil {
				return fmt.Errorf("inserting all networks: %w", err)
			}
		}

		if err := InsertJSON(dbWriter, filepath.Join(w.source, jsonFileName)); err != nil {
			return fmt.Errorf("inserting json: %w", err)
		}

		dbFileName := dbType + "-Test.mmdb"
		if err := w.write(dbWriter, dbFileName); err != nil {
			return fmt.Errorf("writing database: %w", err)
		}
	}

	return nil
}

// InsertJSON reads and parses a json file into mmdbtypes values and inserts
// them into the mmdbwriter tree.
func InsertJSON(dbWriter *mmdbwriter.Tree, filePath string) error {
	file, err := os.Open(filepath.Clean(filePath))
	if err != nil {
		return fmt.Errorf("opening json file: %w", err)
	}
	defer file.Close()

	var data []map[string]any
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return fmt.Errorf("decoding json file: %w", err)
	}

	for _, record := range data {
		for k, v := range record {
			prefix, err := netip.ParsePrefix(k)
			if err != nil {
				return fmt.Errorf("parsing ip: %w", err)
			}

			mmdbValue, err := toMMDBType(prefix.String(), v)
			if err != nil {
				return fmt.Errorf("converting value to mmdbtype: %w", err)
			}

			err = dbWriter.Insert(
				netipx.PrefixIPNet(prefix),
				mmdbValue,
			)
			if err != nil {
				return fmt.Errorf("inserting ip: %w", err)
			}
		}
	}
	return nil
}

// toMMDBType key converts field values read from json into their corresponding mmdbtype.DataType.
// It makes some assumptions for numeric types based on previous knowledge about field types.
func toMMDBType(key string, value any) (mmdbtype.DataType, error) {
	switch v := value.(type) {
	case bool:
		return mmdbtype.Bool(v), nil
	case string:
		return mmdbtype.String(v), nil
	case map[string]any:
		m := mmdbtype.Map{}
		for innerKey, val := range v {
			innerVal, err := toMMDBType(innerKey, val)
			if err != nil {
				return nil, fmt.Errorf("parsing mmdbtype.Map for key %q: %w", key, err)
			}
			m[mmdbtype.String(innerKey)] = innerVal
		}
		return m, nil
	case []any:
		s := mmdbtype.Slice{}
		for _, val := range v {
			innerVal, err := toMMDBType(key, val)
			if err != nil {
				return nil, fmt.Errorf("parsing mmdbtype.Slice for key %q: %w", key, err)
			}
			s = append(s, innerVal)
		}
		return s, nil
	case float64:
		switch key {
		case "accuracy_radius", "anonymizer_confidence", "confidence", "metro_code":
			return mmdbtype.Uint16(v), nil
		case "autonomous_system_number", "average_income",
			"geoname_id", "ipv4_24", "ipv4_32", "ipv6_32",
			"ipv6_48", "ipv6_64", "population_density":
			return mmdbtype.Uint32(v), nil
		case "ip_risk", "latitude", "longitude", "score",
			"static_ip_score":
			return mmdbtype.Float64(v), nil
		default:
			return nil, fmt.Errorf("unsupported numeric type for key %q: %T", key, value)
		}
	default:
		return nil, fmt.Errorf("unsupported type for key %q: %T", key, value)
	}
}

// populate all networks inserts all networks into the writer with an empty map value.
func populateAllNetworks(w *mmdbwriter.Tree) error {
	defaultNet, err := netip.ParsePrefix("::/0")
	if err != nil {
		return fmt.Errorf("parsing ip: %w", err)
	}

	err = w.Insert(
		netipx.PrefixIPNet(defaultNet),
		mmdbtype.Map{},
	)
	if err != nil {
		return fmt.Errorf("inserting ip: %w", err)
	}

	return nil
}
