// based on the https://github.com/umbracle/ethgo/blob/main/abi/decode.go
package abi

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"github.com/umbracle/ethgo"
)

// Decode decodes the input with a given type
func DecodePacked(t *Type, input []byte) (interface{}, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("empty input")
	}
	val, _, err := decodePacked(t, input)
	return val, err
}

func decodePacked(t *Type, input []byte) (interface{}, []byte, error) {
	var err error
	var length int


	switch t.Kind() {
	case KindSlice, KindBytes, KindString:
		length = len(input)
	case KindBool:
		length = 1
	case KindInt, KindUInt:
		length = t.Size()/8
	default:
		length = t.Size()
	}
	if length > len(input) {
		return nil, nil, fmt.Errorf("Input kind '%s' requires length %d, but input has %d", t.Kind(), length, len(input))
	}

	switch t.Kind() {
	case KindTuple:
		return decodeTuplePacked(t, input)

	case KindSlice:
		eSize := t.Elem().Size()
		if eSize == 0 {
			eSize = length
		}
		if t.Elem().Kind() == KindInt || t.Elem().Kind() == KindUInt {
			eSize = eSize/8
		}
		return decodeArraySlicePacked(t, input, length/eSize)

	case KindArray:
		return decodeArraySlicePacked(t, input, t.Size())
	}

	
	var val interface{}
	switch t.Kind() {
	case KindBool:
		val, err = decodeBoolPacked(input[:length])

	case KindInt, KindUInt:
		val = readIntegerPacked(t, input[:length])

	case KindString: // only last bytes
		val = string(input)

	case KindBytes: // only last bytes
		val = input

	case KindAddress:
		val, err = readAddrPacked(input[:length])

	case KindFixedBytes:
		val, err = readFixedBytesPacked(t, input[:length])

	case KindFunction:
		val, err = readFunctionTypePacked(t, input[:length])

	default:
		return nil, nil, fmt.Errorf("decoding not available for type '%s'", t.Kind())
	}

	return val, input[length:], err
}

func readAddrPacked(b []byte) (ethgo.Address, error) {
	res := ethgo.Address{}
	if len(b) != 20 {
		return res, fmt.Errorf("len is not correct")
	}
	copy(res[:], b[:20])
	return res, nil
}

func readIntegerPacked(t *Type, b []byte) interface{} {
	switch t.GoType().Kind() {
	case reflect.Uint8:
		return b[0]

	case reflect.Uint16:
		return binary.BigEndian.Uint16(b)

	case reflect.Uint32:
		return binary.BigEndian.Uint32(b)

	case reflect.Uint64:
		return binary.BigEndian.Uint64(b)

	case reflect.Int8:
		return int8(b[0])

	case reflect.Int16:
		return int16(binary.BigEndian.Uint16(b))

	case reflect.Int32:
		return int32(binary.BigEndian.Uint32(b))

	case reflect.Int64:
		return int64(binary.BigEndian.Uint64(b))

	default:
		ret := new(big.Int).SetBytes(b)
		if t.Kind() == KindUInt {
			return ret
		}

		if ret.Cmp(maxInt256) > 0 {
			ret.Add(maxUint256, big.NewInt(0).Neg(ret))
			ret.Add(ret, big.NewInt(1))
			ret.Neg(ret)
		}
		return ret
	}
}

func readFunctionTypePacked(t *Type, word []byte) ([24]byte, error) {
	res := [24]byte{}
	copy(res[:], word[0:24])
	return res, nil
}

func readFixedBytesPacked(t *Type, word []byte) (interface{}, error) {
	array := reflect.New(t.GoType()).Elem()
	reflect.Copy(array, reflect.ValueOf(word[0:t.Size()]))
	return array.Interface(), nil
}

func decodeTuplePacked(t *Type, data []byte) (interface{}, []byte, error) {
	res := make(map[string]interface{})

	for indx, arg := range t.TupleElems() {

		entry := data

		val, tail, err := decodePacked(arg.Elem, entry)
		if err != nil {
			return nil, nil, err
		}

		data = tail

		name := arg.Name
		if name == "" {
			name = strconv.Itoa(indx)
		}
		if _, ok := res[name]; !ok {
			res[name] = val
		} else {
			return nil, nil, fmt.Errorf("tuple with repeated values")
		}
	}
	return res, data, nil
}

func decodeArraySlicePacked(t *Type, data []byte, size int) (interface{}, []byte, error) {
	if size < 0 {
		return nil, nil, fmt.Errorf("size is lower than zero")
	}
	if t.Elem().Size()/8*size > len(data) {
		return nil, nil, fmt.Errorf("size is too big")
	}

	var res reflect.Value
	if t.Kind() == KindSlice {
		res = reflect.MakeSlice(t.GoType(), size, size)
	} else if t.Kind() == KindArray {
		res = reflect.New(t.GoType()).Elem()
	}

	for indx := 0; indx < size; indx++ {
		entry := data
		val, tail, err := decodePacked(t.Elem(), entry)
		if err != nil {
			return nil, nil, err
		}
		data = tail
		res.Index(indx).Set(reflect.ValueOf(val))
	}
	return res.Interface(), data, nil
}

func decodeBoolPacked(data []byte) (interface{}, error) {
	switch data[0] {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("bad boolean")
	}
}
