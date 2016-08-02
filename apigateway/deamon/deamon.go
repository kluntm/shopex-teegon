package deamon

import (
	"fmt"
	//	l4g "github.com/alecthomas/log4go"
	"os"
	//"runtime"
	"errors"
	"os/exec"
	"os/signal"
	//"path/filepath"
	"syscall"
	"time"
)

func Daemon() int {
	time.Sleep(time.Second * 1)
	c := make(chan os.Signal, 1)
	signal.Notify(c)

	if os.Getppid() != 1 { //判断当其是否是子进程，当父进程return之后，子进程会被 系统1 号进程接管
		filePath := os.Args[0] //将命令行参数中执行文件路径转换成可用路径
		fmt.Printf("cur_pid:%d, ppid:%d, Daemon file path:%s, args:%v\n", os.Getpid(), os.Getppid(), filePath, os.Args[1:])
		cmd := exec.Command(filePath, os.Args[1:]...)
		//将其他命令传入生成出的进程
		/*
			cmd.Stdin = os.Stdin //给新进程设置文件描述符，可以重定向到文件中
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		*/
		cmd.Stdin = nil //给新进程设置文件描述符，可以重定向到文件中
		cmd.Stdout = nil
		cmd.Stderr = nil

		err := cmd.Start() //开始执行新进程，不等待新进程退出
		//创建进程失败则不退出，以当前进程作为监控
		if err != nil {
			fmt.Printf("create process failed, cur_pid:%d, ppid:%d error:%s\n", os.Getpid(), os.Getppid(), err.Error())
			return 0
		}

		fmt.Printf("create process success, cur_pid:%d, new_pid:%d\n", os.Getpid(), cmd.Process.Pid)
		os.Exit(0)
	}
	/*
		var rlimit, zero syscall.Rlimit
		err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit)
		if err != nil {
			fmt.Println("Getrlimit: save failed: %v", err)
			return 0
		}
		if zero == rlimit {
			fmt.Println("Getrlimit: save failed: got zero value %#v", rlimit)
			return 0
		}

		for fd := 0; fd < int(rlimit.Cur); fd++ {
			syscall.Close(fd)
		}
	*/
	return 0

}

func lock_wait(filename string) int {
	//	name := filepath.Join(os.TempDir(), "TestFcntlFlock")
	fd, err := syscall.Open(filename, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		fmt.Printf("open file error:%s\n", err.Error())
	}

	flock := syscall.Flock_t{
		Type:   syscall.F_WRLCK,
		Start:  0,
		Len:    0,
		Whence: 1,
	}

	for {
		if err := syscall.FcntlFlock(uintptr(fd), syscall.F_SETLKW, &flock); err == nil {
			fmt.Printf("FcntlFlock ret:%v, pid:%d, ppid:%d\n", err, os.Getpid(), os.Getppid())
			break
		}
	}
	return 0
}

func Partner(file string, argv []string) error {

	wRet := lock_wait(file)
	if wRet != 0 {
		fmt.Printf("lock file error, file:%s\n", file)
		return errors.New("lock file error, file")
	}

	filePath := os.Args[0] //将命令行参数中执行文件路径转换成可用路径
	fmt.Printf("create child process cur_pid:%d, ppid:%d, path:%s, args:%v\n", os.Getpid(), os.Getppid(), filePath, os.Args[1:])
	cmd := exec.Command(filePath, os.Args[1:]...)
	//将其他命令传入生成出的进程

	cmd.Stdin = nil //给新进程设置文件描述符，可以重定向到文件中
	cmd.Stdout = nil
	cmd.Stderr = nil

	s_err := cmd.Start() //开始执行新进程，不等待新进程退出
	//创建进程失败则不退出，以当前进程作为监控
	if s_err != nil {
		fmt.Printf("create child process failed, cur_pid:%d, ppid:%d error:%s\n", os.Getpid(), os.Getppid(), s_err.Error())
		os.Exit(-1)
		return nil
	}

	go func() {
		cmd.Process.Wait() //等待子进程退出，防止僵死进程
	}()

	time.Sleep(time.Second * 1)

	return nil
	/*
		ret, ret2, err := syscall.RawSyscall(syscall.SYS_FORK, 0, 0, 0)
		if err != 0 {
			fmt.Print("fork error, cur_pid:%d", os.Getpid())
			return errors.New("fork error")
		}

		// failure
		if ret2 < 0 {
			fmt.Print("fork error ret2 < 0, cur_pid:%d", os.Getpid())
			os.Exit(-1)
		}

		// 子进程执行程序,重复上述动作，再lock_wait此处等到父进程退出
		if ret == 0 {
			fmt.Printf("partner exec, pid:%d, ppid:%d, path:%s\n", os.Getpid(), os.Getppid(), argv[0])
			syscall.Exec(argv[0], argv, nil)
			fmt.Printf("partner exec success, pid:%d, ppid:%d, path:%s\n", os.Getpid(), os.Getppid(), argv[0])

		} else {
			p, err := os.FindProcess(int(ret))
			if err == nil {
				go func() {
					p.Wait() //等待子进程退出，防止僵死进程
				}()
			}
		}

		time.Sleep(time.Second * 1)

		return nil
	*/
}
