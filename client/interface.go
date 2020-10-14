package client

import (
	"github.com/eden-framework/courier"
)

type IRequest interface {
	Do() courier.Result
}
