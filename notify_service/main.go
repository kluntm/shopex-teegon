package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	//"git.ishopex.cn/teegon/apigateway/deamon"
	l4g "github.com/alecthomas/log4go"
	//"os/exec"
	"os/signal"
	"runtime"
	//	"runtime/debug"
	"syscall"
)

const (
	MajorVersion = "1"
	MinorVersion = "0"
)

var cmdVersion = flag.Bool("version", false, "show version information")
var gApp *ServiceApp

func UnInit() {
	if err := recover(); err != nil {
		l4g.Error("runtime error:", err) //这里的err其实就是panic传入的内容，55
		l4g.Error("stack", string(debug.Stack()))
	}

	if gApp != nil {
		gApp.UnInit()
	}

	l4g.Close()
}
func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	confPath := flag.String("f", "./etc/gateway.cfg", "cm config")

	flag.Parse()

	if *cmdVersion {
		fmt.Printf("%s.%s.%s\n", MajorVersion, MinorVersion, BuildVersion)
		return
	}

	fmt.Println(*confPath)
	if len(*confPath) == 0 {
		fmt.Errorf("not config file.")
		return
	}
	defer UnInit()

	gApp := &ServiceApp{}
	if err := gApp.LoadConfig(*confPath); err != nil {
		fmt.Errorf("load config failed, error:%s", err.Error())
		return
	}
	if gApp.config.Daemon {
		//deamon.Daemon()
		//deamon.Partner("./etc/parent.pid", os.Args)
	}

	if gApp.Init() != nil {
		fmt.Errorf("init failed, exit.")
		//os.Exit(0)
		return
	}

	endch := make(chan int)

	c := make(chan os.Signal, 1)

	//chldCh := make(chan os.Signal, 1)
	//signal.Notify(c)
	signal.Notify(c, os.Kill, os.Interrupt, syscall.SIGHUP)

	go func() {
		for sig := range c {

			switch sig {
				case os.Kill:
					l4g.Info("received ctrl+c (%s)\n", sig.String())
					endch <- 1
					return
				case os.Interrupt:
					l4g.Info("received Interrupt (%s)\n", sig.String())
				default:
					break
			}

		}
	}()

	<-endch

	//start.UnInit()

	l4g.Info("end!")
}
