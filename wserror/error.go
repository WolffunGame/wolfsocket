package wserror

import (
	"errors"
	"strconv"
	"strings"
)

type ErrorCode int64

const (
	DefaultErr     ErrorCode = -99
	AlreadyInParty ErrorCode = 1
)

func (errorCode ErrorCode) WSErr(msgs ...string) WSError {
	return New(errorCode, strings.Join(msgs, ", "))
}

type WSError struct {
	errorCode ErrorCode
	errMsg    string
}

func New(code ErrorCode, msg string) WSError {
	return WSError{code, msg}
}

func Error(err error) WSError {
	var wsErr WSError
	if errors.As(err, &wsErr) {
		return wsErr
	}
	return WSError{DefaultErr, err.Error()}
}

// @Tinh note: thống nhât giữa Unity và BE là dùng ErrorCode để giao tiếp khi có lỗi
// - giảm bytes gửi - nhận trong Message
// - cụ thể hóa từng error, và dùng lại ở 1 số nơi khác khi cùng lỗi

// error
func (err WSError) Error() string {
	return strconv.Itoa(int(err.errorCode))
}

func (err WSError) ErrorCode() int64 {
	return int64(err.errorCode)
}

// stringer
func (err WSError) String() string {
	return err.errMsg
}

func (err WSError) Bytes() []byte {
	return []byte(err.Error())
}
