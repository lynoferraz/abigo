// based on the https://github.com/umbracle/ethgo/blob/main/abi/encode.go
package abi

import (
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"github.com/umbracle/ethgo"
)

// Encode encodes a value
func EncodePacked(v interface{}, t *Type) ([]byte, error) {
	return encodePacked(reflect.ValueOf(v), t)
}

func encodePacked(v reflect.Value, t *Type) ([]byte, error) {
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}

	switch t.Kind() {
	case KindSlice, KindArray:
		return encodeSliceAndArrayPacked(v, t)

	case KindTuple:
		return encodeTuplePacked(v, t)

	case KindString:
		return encodeStringPacked(v)

	case KindBool:
		return encodeBoolPacked(v)

	case KindAddress:
		return encodeAddressPacked(v)

	case KindInt, KindUInt:
		return encodeNumPacked(v,t)

	case KindBytes:
		return encodeBytesPacked(v)

	case KindFixedBytes, KindFunction:
		return encodeFixedBytesPacked(v,t)

	default:
		return nil, fmt.Errorf("encoding not available for type '%s'", t.Kind())
	}
}

func encodeSliceAndArrayPacked(v reflect.Value, t *Type) ([]byte, error) {
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		return nil, encodeErr(v, t.Kind().String())
	}

	if v.Kind() == reflect.Array && t.Kind() != KindArray {
		return nil, fmt.Errorf("expected array")
	} else if v.Kind() == reflect.Slice && t.Kind() != KindSlice {
		return nil, fmt.Errorf("expected slice")
	}

	if t.Kind() == KindArray && t.Size() != v.Len() {
		return nil, fmt.Errorf("array len incompatible")
	}

	var ret, tail []byte

	for i := 0; i < v.Len(); i++ {
		val, err := encodePacked(v.Index(i), t.Elem())
		if err != nil {
			return nil, err
		}
		ret = append(ret, val...)
	}
	return append(ret, tail...), nil
}

func encodeTuplePacked(v reflect.Value, t *Type) ([]byte, error) {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	var err error
	isList := true

	switch v.Kind() {
	case reflect.Slice, reflect.Array:
	case reflect.Map:
		isList = false

	case reflect.Struct:
		isList = false
		v, err = mapFromStruct(v)
		if err != nil {
			return nil, err
		}

	default:
		return nil, encodeErr(v, "tuple")
	}

	if v.Len() < len(t.TupleElems()) {
		return nil, fmt.Errorf("expected at least the same length")
	}

	var ret, tail []byte
	var aux reflect.Value

	for i, elem := range t.TupleElems() {
		if isList {
			aux = v.Index(i)
		} else {
			name := elem.Name
			if name == "" {
				name = strconv.Itoa(i)
			}
			aux = v.MapIndex(reflect.ValueOf(name))
		}
		if aux.Kind() == reflect.Invalid {
			return nil, fmt.Errorf("cannot get key %s", elem.Name)
		}

		val, err := encodePacked(aux, elem.Elem)
		if err != nil {
			return nil, err
		}
		ret = append(ret, val...)
	}

	return append(ret, tail...), nil
}

func encodeFixedBytesPacked(v reflect.Value, t *Type) ([]byte, error) {
	if v.Kind() == reflect.Array {
		v = convertArrayToBytes(v)
	}
	if v.Kind() == reflect.String {
		value, err := decodeHex(v.String())
		if err != nil {
			return nil, err
		}

		v = reflect.ValueOf(value)
	}
	return rightPad(v.Bytes(), t.Size()), nil
}

func encodeAddressPacked(v reflect.Value) ([]byte, error) {
	if v.Kind() == reflect.Array {
		v = convertArrayToBytes(v)
	}
	if v.Kind() == reflect.String {
		var addr ethgo.Address
		if err := addr.UnmarshalText([]byte(v.String())); err != nil {
			return nil, err
		}
		v = reflect.ValueOf(addr.Bytes())
	}
	return v.Bytes(), nil
}

func encodeBytesPacked(v reflect.Value) ([]byte, error) {
	if v.Kind() == reflect.Array {
		v = convertArrayToBytes(v)
	}
	if v.Kind() == reflect.String {
		value, err := decodeHex(v.String())
		if err != nil {
			return nil, err
		}

		v = reflect.ValueOf(value)
	}
	return v.Bytes(), nil
}

func encodeStringPacked(v reflect.Value) ([]byte, error) {
	if v.Kind() != reflect.String {
		return nil, encodeErr(v, "string")
	}
	return []byte(v.String()), nil
}

func encodeNumPacked(v reflect.Value, t *Type) ([]byte, error) {
	switch v.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return toUSize(new(big.Int).SetUint64(v.Uint()),t.Size()), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return toUSize(big.NewInt(v.Int()),t.Size()), nil

	case reflect.Ptr:
		if v.Type() != bigIntT {
			return nil, encodeErr(v.Elem(), "number")
		}
		return toUSize(v.Interface().(*big.Int),256), nil

	case reflect.Float64:
		return encodeNumPacked(reflect.ValueOf(int64(v.Float())),t)

	case reflect.String:
		n, ok := new(big.Int).SetString(v.String(), 10)
		if !ok {
			n, ok = new(big.Int).SetString(v.String()[2:], 16)
			if !ok {
				return nil, encodeErr(v, "number")
			}
		}
		return encodeNumPacked(reflect.ValueOf(n),t)

	default:
		return nil, encodeErr(v, "number")
	}
}

func encodeBoolPacked(v reflect.Value) ([]byte, error) {
	if v.Kind() != reflect.Bool {
		return nil, encodeErr(v, "bool")
	}
	if v.Bool() {
		return one.Bytes(), nil
	}
	return zero.Bytes(), nil
}

func toUSize(n *big.Int, size int) []byte {
	b := new(big.Int)
	b = b.Set(n)

	if b.Sign() < 0 || b.BitLen() > size {
		tt   := new(big.Int).Lsh(big.NewInt(1), uint(size))   // 2 ** 256
		ttm1 := new(big.Int).Sub(tt, big.NewInt(1)) // 2 ** 256 - 1
		b.And(b, ttm1)
	}

	return leftPad(b.Bytes(), size / 8)
}
