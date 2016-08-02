package lib

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

var ErrorHandler *error_handler

type error_handler struct {
	fd               *os.File
	last_err_count   int
	last_report_time time.Time
	lk               sync.Mutex
	re               *regexp.Regexp
	last             *[]byte
}

func init() {
	ErrorHandler = &error_handler{}
	ErrorHandler.Init()

}

func (me *error_handler) Init() {
	var err error
	error_file := "logs/error.log"
	me.fd, err = os.OpenFile(error_file, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0664)
	if err != nil {
		log.Printf("[error] open crashlog %s, error: %s", error_file, err)
		me.fd = os.Stderr
	}
	me.re = regexp.MustCompile("\\(0x[0-9a-f]+\\)")
}

func (me *error_handler) On(err interface{}) {
	me.lk.Lock()
	defer me.lk.Unlock()

	stack := debug.Stack()
	stack = me.re.ReplaceAll(stack, []byte(""))
	me.fd.WriteString(fmt.Sprintf("\n%s %s\nError: %s\nTrace:", time.Now(), strings.Repeat("-", 50), err))
	me.fd.Write(stack)
	me.last = &stack
}
