package handler

import (
	"fmt"
	"github.com/wang1137095129/kubernetes-with-cluster/event"
)

type Handler interface {
	Init() error
	Handle(event event.Event)
}

type Default struct {
}

func (d Default) Init() error {
	return nil
}

func (d Default) Handle(event event.Event) {
	fmt.Printf("The %s type of %s in namespace:%s got a %s with %s", event.Name, event.Kind, event.Namespace, event.Status, event.Reason)
}
