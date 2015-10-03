package main

import (
	"./bmemcached"
)

func main() {
	server := bmemcached.NewServer()
	server.MainLoop()
}