package lib

import (
	"encoding/json"
	"fmt"
)

type Error struct {
	Code     string
	Message  string
	HttpCode int
}

func (e *Error) MarshalJSON() ([]byte, error) {
	tv := map[string]interface{}{
		"code":    e.Code,
		"message": e.Message,
	}
	return json.Marshal(tv)
}

func (e Error) Error() string {
	return fmt.Sprintf("code:%s, message:%s", e.Code, e.Message)
}

type ErrList map[string]Error

func NewError(err interface{}) *Error {
	return &Error{
		Code:     "0040000",
		HttpCode: 400,
		Message:  fmt.Sprintf("%s", err),
	}
}

func (l ErrList) Add(e Error) {
	(map[string]Error)(l)[e.Code] = e
}

func (l ErrList) Get(code string, v interface{}) (errdata Error) {
	errdata = (map[string]Error)(l)[code]
	if v != nil {
		errdata.Message = fmt.Sprintf(errdata.Message, v)
	}
	return
}

var Errors ErrList

func init() {
	Errors = ErrList(map[string]Error{})
	Errors.Add(Error{"0050400", "backend error", 504})

	Errors.Add(Error{"0040400", "api not exists", 404})
	Errors.Add(Error{"0040401", "method not exists", 404})
	Errors.Add(Error{"0040005", "missing param '%s'", 404})
	Errors.Add(Error{"0040301", "access denied, sign error", 403})
	Errors.Add(Error{"0040302", "access denied, app_key error", 404})
	Errors.Add(Error{"0040303", "access denied, key expired", 403})
	Errors.Add(Error{"0040304", "access denied, secret error", 403})
	Errors.Add(Error{"0040305", "access denied, app error", 403})
	Errors.Add(Error{"0040306", "access denied, app is disabled", 403})
	Errors.Add(Error{"0040307", "access denied, sign_time format error", 403})
	Errors.Add(Error{"0040308", "access denied, sign_time is out of date", 403})
	Errors.Add(Error{"0040309", "access denied, app can not call the api", 403})
	Errors.Add(Error{"0040310", "access denied, oauth access token required", 403})
	Errors.Add(Error{"0040311", "access denied, out of limit, waiting for %d seconds", 403})
	Errors.Add(Error{"0040312", "access denied, username password are not match", 403})
}

type ApiError struct {
	Code  string
	Value interface{}
}

var Api_Not_Exists = Error{"0040400", "api not exists", 404}
var Backend_Error = Error{"0050400", "backend error", 504}
var Method_Not_Exists = Error{"0040401", "method not exists", 404}
var Param_Error = Error{"0040005", "missing param '%s'", 404}
var Sign_Error = Error{"0040301", "access denied, sign error", 403}
var ClientId_Error = Error{"0040302", "access denied, app_key error", 404}
var KeyExpired_Error = Error{"0040303", "access denied, key expired", 403}
var Secret_Error = Error{"0040304", "access denied, secret error", 403}
var AppError = Error{"0040305", "access denied, app error", 403}
var AppDisabled_Error = Error{"0040306", "access denied, app is disabled", 403}
var SignTime_Error = Error{"0040307", "access denied, sign_time format error", 403}
var SingTimeOutOfDate_Error = Error{"0040308", "access denied, sign_time is out of date", 403}
var ApiCall_Permis_Error = Error{"0040309", "access denied, app can not call the api", 403}
var ApiCall_Limit_Error = Error{"0040311", "access denied, out of limit, waiting for %d seconds", 403}
