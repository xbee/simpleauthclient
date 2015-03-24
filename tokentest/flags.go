package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	flTls  = flag.Bool("-tls", false, "enable daemon mode")
	flHost = flag.String("-host", "", "set host")
)

func init() {
	flag.Usage = func() {
		fmt.Fprint(os.Stdout, "Usage: tokentest [OPTIONS] COMMAND [arg...]\n\nOptions:\n")
		flag.PrintDefaults()
		flag.CommandLine.SetOutput(os.Stdout)
		help := `Commands:
            reset:      restart service
            login:      get a token of access
            wlecome:    welcome`
		fmt.Fprint(os.Stdout, help)
	}
}
