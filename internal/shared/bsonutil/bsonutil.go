package bsonutil

import (
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

// StructToBsonM converts a struct to bson.M using the struct's bson tags as keys.
// Fields listed in skip (by bson tag name) are excluded.
// Fields with bson tag "-" or no bson tag are always excluded.
func StructToBsonM(v any, skip ...string) bson.M {
	skipSet := make(map[string]struct{}, len(skip))
	for _, s := range skip {
		skipSet[s] = struct{}{}
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	rt := rv.Type()

	m := make(bson.M, rt.NumField())
	for i := range rt.NumField() {
		field := rt.Field(i)
		tag := field.Tag.Get("bson")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.SplitN(tag, ",", 2)[0]
		if name == "-" || name == "" {
			continue
		}
		if _, skip := skipSet[name]; skip {
			continue
		}
		m[name] = rv.Field(i).Interface()
	}
	return m
}
