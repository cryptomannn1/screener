package backtest

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
)

// JSON encoding: encoding/json rejects NaN, and a cell with no observations
// is all NaN. Both result structs marshal through nanSafe, which emits null
// for a NaN or infinite float and leaves everything else to the default
// encoder (field names come from the json tags).

// MarshalJSON implements json.Marshaler.
func (c Cell) MarshalJSON() ([]byte, error) { return json.Marshal(nanSafe(c)) }

// MarshalJSON implements json.Marshaler.
func (p PresetResult) MarshalJSON() ([]byte, error) { return json.Marshal(nanSafe(p)) }

func nanSafe(v any) map[string]any {
	rv := reflect.ValueOf(v)
	rt := rv.Type()
	out := make(map[string]any, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" || f.PkgPath != "" {
			continue
		}
		name := f.Name
		if tag != "" {
			if n := strings.Split(tag, ",")[0]; n != "" {
				name = n
			}
		}
		fv := rv.Field(i)
		if fv.Kind() == reflect.Float64 {
			if x := fv.Float(); math.IsNaN(x) || math.IsInf(x, 0) {
				out[name] = nil
				continue
			}
		}
		out[name] = fv.Interface()
	}
	return out
}
