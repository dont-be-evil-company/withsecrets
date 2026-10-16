//go:build !wails

package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/mistweaverco/withsecrets/internal/vaulthardware/doctor"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		rep := doctor.Inspect(false, false)
		fmt.Printf("os=%s fido2=%v piv=%v\n", rep.OS, rep.FIDO2Tag, rep.PIVTag)
		for _, c := range rep.Checks {
			fmt.Printf("%s ok=%v %s\n", c.Name, c.OK, c.Detail)
		}
		return
	}
	addr := "127.0.0.1:11912"
	if len(os.Args) > 1 && os.Args[1] == "--addr" && len(os.Args) > 2 {
		addr = os.Args[2]
	}
	fmt.Printf("withsecrets desktop (dev shell) http://%s\n", addr)
	fmt.Println("Native Wails shell: go build -tags wails ./apps/desktop")
	host := newHost()
	host.watchIdle()
	if err := http.ListenAndServe(addr, host.routes()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
