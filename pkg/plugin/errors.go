package plugin

import "github.com/pkg/errors"

var (
	ErrorMessageInvalidJSON       = errors.New("could not parse json")
	ErrorMessageInvalidServerName = errors.New("invalid server name. Either empty or not set")
	ErrorMessageInvalidPort       = errors.New("invalid port")
	ErrorMessageInvalidUserName   = errors.New("username is either empty or not set")
	ErrorMessageInvalidPassword   = errors.New("password is either empty or not set")
)
