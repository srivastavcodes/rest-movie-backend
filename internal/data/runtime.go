package data

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidRuntimeFormat = errors.New("invalid runtime format")

type Runtime int32

//goland:noinspection GoMixedReceiverTypes
func (r Runtime) MarshalJSON() ([]byte, error) {
	length := fmt.Sprintf("%d mins", r)
	quotedLength := strconv.Quote(length)
	return []byte(quotedLength), nil
}

//goland:noinspection GoMixedReceiverTypes
func (r *Runtime) UnmarshalJSON(length []byte) error {
	unquotedLength, err := strconv.Unquote(string(length))
	if err != nil {
		return ErrInvalidRuntimeFormat
	}
	parts := strings.Split(unquotedLength, " ")
	if len(parts) != 2 || parts[1] != "mins" {
		return ErrInvalidRuntimeFormat
	}
	val, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return ErrInvalidRuntimeFormat
	}
	*r = Runtime(val)
	return nil
}
