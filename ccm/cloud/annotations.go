package cloud

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
)

// stringToMapHookFunc returns a DecodeHookFunc that converts
// a key=val,key=val string to a map[string]string
func stringToMapHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Kind,
		t reflect.Kind,
		data interface{}) (interface{}, error) {
		if f != reflect.String || t != reflect.Map {
			return data, nil
		}

		raw, ok := data.(string)
		if raw == "" || !ok {
			return map[string]string{}, nil
		}

		list := strings.Split(raw, ",")

		m := map[string]string{}
		// Break up "Key=Val"
		for _, kvs := range list {
			kv := strings.Split(strings.TrimSpace(kvs), "=")
			switch len(kv) {
			case 1:
				m[kv[0]] = ""
			case 2:
				m[kv[0]] = kv[1]
			default:
				return nil, fmt.Errorf("invalid value %q", raw)
			}
		}

		return m, nil
	}
}

// matchAnnotationName matches annotations with annotation struct tag values.
func matchAnnotationName(annotationName, fieldName string) bool {
	annotationName = strings.TrimPrefix(annotationName, "service.beta.kubernetes.io/")
	if strings.EqualFold(annotationName, fieldName) {
		return true
	}
	oscAnnotation := strings.Replace(annotationName, "aws-", "osc-", 1)
	oscAnnotation = strings.Replace(oscAnnotation, "-s3-", "-oos-", 1)
	return strings.EqualFold(oscAnnotation, fieldName)
}
