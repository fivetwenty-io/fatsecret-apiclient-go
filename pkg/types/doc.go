// Package types provides tolerant scalar types for decoding FatSecret API JSON responses.
//
// FatSecret returns all numeric fields as quoted JSON strings (e.g. "123", "29.55"),
// uses "0"/"1" string integers for booleans, "-1"/"0"/"1" for ternary allergen values,
// and encodes dates as integer days since 1970-01-01 in quoted strings. Many v1 endpoints
// also collapse a single-element collection to a bare JSON object instead of a one-element
// array.
//
// The types in this package implement [encoding/json.Unmarshaler] and [encoding/json.Marshaler]
// to handle every observed wire form transparently. Consumer code can declare struct fields
// using these types and rely on standard [encoding/json.Unmarshal] without any pre-processing.
//
// All types are safe for concurrent use once constructed; pointer receivers are required only
// for UnmarshalJSON (as the standard library expects).
package types
