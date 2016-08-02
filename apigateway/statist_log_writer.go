package main

import (
	"fmt"
	"os"
	"time"

	l4g "github.com/alecthomas/log4go"
)

type StatistLogWriter struct {
	file           *os.File
	filename       string
	daily_opendate int
	maxbackup      int
	rotate         bool
}

var StatistLog *StatistLogWriter

func NewStatistLog() *StatistLogWriter {
	if StatistLog != nil {
		return StatistLog
	}

	StatistLog = &StatistLogWriter{}
	return StatistLog
}

func (log *StatistLogWriter) Init(filename string) {
	log.filename = filename
	log.maxbackup = 999
	log.rotate = true
}

func (w *StatistLogWriter) intRotate() error {
	// Close any log file that may be open
	if w.file != nil {
		w.file.Close()
	}

	// If we are keeping log files, move it to the next available number
	_, err := os.Lstat(w.filename)
	if err == nil { // file exists
		// Find the next available number
		num := 1
		fname := ""
		if time.Now().Day() != w.daily_opendate {
			yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

			for ; err == nil && num <= 999; num++ {
				fname = w.filename + fmt.Sprintf(".%s.%03d", yesterday, num)
				_, err = os.Lstat(fname)
			}
			// return error if the last file checked still existed
			if err == nil {
				return fmt.Errorf("Rotate: Cannot find free log number to rename %s\n", w.filename)
			}
		}

		if w.file != nil {
			w.file.Close()
		}

		// Rename the file to its newfound home
		err = os.Rename(w.filename, fname)
		if err != nil {
			return fmt.Errorf("Rotate: %s\n", err)
		}
	}

	// Open the log file
	fd, err := os.OpenFile(w.filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
	if err != nil {
		return err
	}
	w.file = fd

	now := time.Now()
	// Set the daily open date to the current date
	w.daily_opendate = now.Day()

	return nil
}
func (log *StatistLogWriter) getFileHandle() (*os.File, error) {
	now := time.Now()
	l4g.Warn("write filename:%s, opdate:%d, date:%d", log.filename, log.daily_opendate, now.Day())

	if now.Day() != log.daily_opendate {
		if err := log.intRotate(); err != nil {

			return nil, err
		}
	}
	return log.file, nil
}

func (log StatistLogWriter) LogWrite(rec *l4g.LogRecord) {
	//l4g.Info("logwrite:%v", rec)
	if rec != nil && rec.Source == "statist_log" {
		logFile, err := StatistLog.getFileHandle()
		if err != nil {
			l4g.Warn("write statist log failed, open file error, file:%s, error:%s,", log.filename, err.Error())
			return
		}
		l4g.Info("write statislog")
		fmt.Fprint(logFile, rec.Message, "\n")
	}
}

func (log StatistLogWriter) Close() {
	if log.file != nil {
		log.file.Close()
	}
}
