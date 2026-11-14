package main

import (
	"os"
	"reflect"
)

// pathRows lists every non-empty string field of a vendor Paths struct with
// whether the path exists. Disk · Read (one stat per path). reflect is used
// only to walk field names and string values, so a new vendor path shows up
// here without editing the example.
func pathRows(v reflect.Value) []PathRow {
	t := v.Type()
	var out []PathRow
	for i := range t.NumField() {
		f := v.Field(i)
		if !t.Field(i).IsExported() || f.Kind() != reflect.String || f.String() == "" {
			continue
		}
		out = append(out, PathRow{Name: t.Field(i).Name, Path: f.String(), Exists: fileExists(f.String())})
	}
	return out
}

// fileExists stats a path. Disk · Read.
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
