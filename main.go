package main

import (
	"flag"
	"github.com/wang1137095129/kubernetes-with-cluster/controller"
	"github.com/wang1137095129/kubernetes-with-cluster/handler"
)

func main() {
	s := flag.String("namespace", "default", "watch's name space")
	flag.Parse()
	controller.Start(handler.Default{}, *s)
}
