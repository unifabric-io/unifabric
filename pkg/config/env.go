// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
)

func applyEnvTags(target any) error {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("env target must be a pointer to struct")
	}
	return applyEnvTagsToStruct(value.Elem())
}

func applyEnvTagsToStruct(value reflect.Value) error {
	valueType := value.Type()
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		fieldType := valueType.Field(index)

		if field.Kind() == reflect.Struct && field.CanAddr() {
			if err := applyEnvTagsToStruct(field); err != nil {
				return err
			}
		}

		key := fieldType.Tag.Get("env")
		if key == "" {
			continue
		}
		raw, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		if err := setEnvTaggedField(field, key, raw); err != nil {
			return err
		}
	}
	return nil
}

func setEnvTaggedField(field reflect.Value, key, raw string) error {
	if !field.CanSet() {
		return fmt.Errorf("%s: field is not settable", key)
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("%s: %q is invalid, expect true or false", key, raw)
		}
		field.SetBool(value)
	case reflect.Ptr:
		if field.Type().Elem().Kind() != reflect.Bool {
			return fmt.Errorf("%s: unsupported pointer field type %s", key, field.Type())
		}
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("%s: %q is invalid, expect true or false", key, raw)
		}
		ptr := reflect.New(field.Type().Elem())
		ptr.Elem().SetBool(value)
		field.Set(ptr)
	default:
		return fmt.Errorf("%s: unsupported field type %s", key, field.Type())
	}
	return nil
}
